package scanner

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync/atomic"
	"time"

	"dfipam/api/internal/modules/monitoring"
	"dfipam/api/internal/netcalc"
)

const maxSweepHosts = 4096

// errTooLarge is a segment the scanner never sweeps by design. It is not a
// failed scan, so its alerts (from FortiGate evidence) still close as usual.
var errTooLarge = fmt.Errorf("network has more than %d scannable addresses; narrow the segment", maxSweepHosts)

type segment struct {
	id, name, network, kind string
	dhcpStart, dhcpEnd      string
	gateway                 string
	checkPorts              bool
}

type sighting struct {
	ip, mac, hostname, source, vendor string
	nicVendor, maker, model, kind, os string
	static, gateway, portsChecked     bool
	full                              bool // identified in full this time
	ports                             []int
}

func (s *Scanner) loadSegments(ctx context.Context, only string) ([]segment, error) {
	rows, err := s.conn.Query(ctx, `SELECT id::text, name, network::text, kind, COALESCE(host(dhcp_start), ''), COALESCE(host(dhcp_end), ''), COALESCE(host(gateway), ''), check_ports
		FROM segments WHERE kind <> 'cloud' AND ($1 = '' OR id::text = $1) ORDER BY kind, floor NULLS LAST, name`, only)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []segment
	for rows.Next() {
		var x segment
		if err := rows.Scan(&x.id, &x.name, &x.network, &x.kind, &x.dhcpStart, &x.dhcpEnd, &x.gateway, &x.checkPorts); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

// scanAll sweeps the segments one after another, records how each went, then
// evaluates the alert rules once.
func (s *Scanner) scanAll(ctx context.Context, req request) error {
	segments, err := s.loadSegments(ctx, req.segmentID)
	if err != nil {
		return err
	}
	if req.by != "manual" {
		segments = s.automatic(ctx, segments)
	}
	started := time.Now()
	s.mu.Lock()
	s.status.Running, s.status.Trigger, s.status.StartedAt, s.status.NextScanAt = true, req.by, &started, nil
	s.mu.Unlock()
	defer func() {
		done := time.Now()
		s.mu.Lock()
		s.status.Running, s.status.FinishedAt = false, &done
		s.mu.Unlock()
	}()
	var scanErrors []error
	for _, x := range segments {
		if err := ctx.Err(); err != nil {
			return err
		}
		begin := time.Now()
		found, err := s.scanSegment(ctx, x, req.by == "manual")
		result := SegmentResult{ID: x.id, Name: x.name, Found: found, DurationMs: time.Since(begin).Milliseconds(), ScannedAt: begin}
		if err != nil {
			result.Error = err.Error()
			scanErrors = append(scanErrors, fmt.Errorf("segment %q: %w", x.name, err))
		}
		s.record(result)
		if ctx.Err() == nil && !errors.Is(err, errTooLarge) {
			if err := s.markScanned(ctx, x.id, err != nil); err != nil {
				scanErrors = append(scanErrors, err)
			}
		}
		// Devices found here become pending and raise their alerts now, rather
		// than after the slowest segments in the sweep.
		if err == nil && found > 0 {
			if err := monitoring.New(s.conn).Evaluate(ctx); err != nil {
				scanErrors = append(scanErrors, err)
			}
		}
	}
	if err := monitoring.New(s.conn).Evaluate(ctx); err != nil {
		scanErrors = append(scanErrors, err)
	}
	// Evidence is kept for 90 days after a device was last seen. Phones that
	// change their private MAC would otherwise leave rows behind forever.
	// Closed alerts are kept for a year, for the Alert history report.
	if _, err := s.conn.Exec(ctx, `DELETE FROM sightings WHERE last_seen < now() - interval '90 days'`); err != nil {
		scanErrors = append(scanErrors, err)
	}
	if _, err := s.conn.Exec(ctx, `DELETE FROM alerts WHERE status = 'closed' AND closed_at < now() - interval '365 days'`); err != nil {
		scanErrors = append(scanErrors, err)
	}
	return errors.Join(scanErrors...)
}

// markScanned records whether a segment's scan failed, writing only when that
// changes. The alert rules keep a failed segment's alerts open.
func (s *Scanner) markScanned(ctx context.Context, segmentID string, failed bool) error {
	_, err := s.conn.Exec(ctx, `UPDATE segments SET scan_failed_at = CASE WHEN $2 THEN now() END
		WHERE id = $1::uuid AND (scan_failed_at IS NULL) = $2`, segmentID, failed)
	return err
}

// automatic keeps the segments whose kind is scanned on schedule. Scan now
// scans any segment, whatever these switches say.
func (s *Scanner) automatic(ctx context.Context, all []segment) []segment {
	on := map[string]bool{"dhcp": enabled(ctx, s.conn, "scan.auto_devices"), "server": enabled(ctx, s.conn, "scan.auto_servers")}
	out := all[:0]
	for _, x := range all {
		if on[x.kind] {
			out = append(out, x)
		}
	}
	return out
}

// record keeps the latest result per segment.
func (s *Scanner) record(r SegmentResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, x := range s.status.Segments {
		if x.ID == r.ID {
			s.status.Segments[i] = r
			return
		}
	}
	s.status.Segments = append(s.status.Segments, r)
}

func scanHosts(network string) ([]netip.Addr, error) {
	prefix, err := netip.ParsePrefix(network)
	if err != nil {
		return nil, err
	}
	span := netcalc.UsableRange(prefix)
	if span.Size().Int64() > maxSweepHosts {
		return nil, errTooLarge
	}
	hosts := make([]netip.Addr, 0, int(span.Size().Int64()))
	for ip := span.First; ip.IsValid() && !span.Last.Less(ip); ip = ip.Next() {
		hosts = append(hosts, ip)
	}
	return hosts, nil
}

// scanSegment sweeps one segment. A manual scan identifies hosts in full more
// often than a scheduled one (see dueFull).
func (s *Scanner) scanSegment(ctx context.Context, seg segment, manual bool) (int, error) {
	hosts, err := scanHosts(seg.network)
	if err != nil {
		return 0, err
	}
	icmp, err := newPinger()
	if err != nil {
		return 0, err
	}
	defer icmp.Close()
	tcpTicker := time.NewTicker(tcpEvery)
	defer tcpTicker.Stop()
	var sendErrors atomic.Int64
	var firstError atomic.Value
	plan := probePlan{seg: seg, hostGateway: defaultGateway(), pinger: icmp, portSlots: make(chan struct{}, portSlots), tcpTick: tcpTicker.C,
		due: func(ip string) bool { return s.dueFull(seg, ip, manual) }, done: func(ip string) { s.markFull(seg, ip) },
		sendErrors: &sendErrors, firstError: &firstError}
	found := sweep(ctx, plan, hosts)
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	found = addNeighbours(ctx, seg, hosts, found, arpTable(), ownMACs())
	if len(found) > 0 {
		if err := s.saveSightings(ctx, seg.id, found); err != nil {
			return 0, err
		}
	}
	if n := sendErrors.Load(); n > 0 {
		return len(found), fmt.Errorf("%d pings could not be sent (%v); the results may be incomplete", n, firstError.Load())
	}
	return len(found), nil
}
