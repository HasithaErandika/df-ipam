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
	workgroup                         string      // NetBIOS workgroup or Windows domain
	windows                           windowsInfo // from the SMB handshake, when it answered
	static, gateway, portsChecked     bool
	full                              bool // identified in full this time
	ports                             []int
}

// sweepable limits sweeps to private address space (RFC 1918, and RFC 6598
// carrier-grade NAT). A floor or server segment typed with a public range is
// never swept, so the scanner can't be pointed at someone else's network.
const sweepable = `(network <<= '10.0.0.0/8' OR network <<= '172.16.0.0/12' OR network <<= '192.168.0.0/16' OR network <<= '100.64.0.0/10')`

func (s *Scanner) loadSegments(ctx context.Context, only string) ([]segment, error) {
	rows, err := s.conn.Query(ctx, `SELECT id::text, name, network::text, kind, COALESCE(host(dhcp_start), ''), COALESCE(host(dhcp_end), ''), COALESCE(host(gateway), ''), check_ports
		FROM segments WHERE kind NOT IN ('cloud', 'vpn') AND `+sweepable+` AND ($1 = '' OR id::text = $1) ORDER BY kind, floor NULLS LAST, name`, only)
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
	if req.segmentID == "" {
		segments = only(segments, req.kinds)
	}
	s.pace.reset(number(ctx, s.conn, "scan.rate_per_second", DefaultRate))
	s.fullMu.Lock()
	s.portGap = interval(ctx, s.conn, "scan.port_check_hours", DefaultPortEvery)
	s.fullMu.Unlock()
	run, end := s.begin(ctx, req.by)
	defer end()
	var scanErrors []error
	for _, x := range segments {
		if run.Err() != nil {
			break
		}
		scanErrors = append(scanErrors, s.sweepOne(ctx, run, x, req.by == "manual")...)
	}
	// A stop ends the sweeps only: what was saved is still evaluated.
	if err := ctx.Err(); err != nil {
		return err
	}
	return errors.Join(append(scanErrors, s.afterScan(ctx)...)...)
}

// sweepOne scans one segment under run, which Stop cancels, and records how it
// went. Database work after the sweep uses ctx, so a stop doesn't cut it short.
func (s *Scanner) sweepOne(ctx, run context.Context, x segment, manual bool) []error {
	begin := time.Now()
	found, err := s.scanSegment(run, x, manual)
	result := SegmentResult{ID: x.id, Name: x.name, Found: found, DurationMs: time.Since(begin).Milliseconds(), ScannedAt: begin}
	if run.Err() != nil && ctx.Err() == nil { // stopped by hand: neither a result nor a failure
		result.Found, result.Error = 0, stoppedText
		s.record(result)
		return nil
	}
	var out []error
	if err != nil {
		result.Error = err.Error()
		out = append(out, fmt.Errorf("segment %q: %w", x.name, err))
	}
	s.record(result)
	if ctx.Err() == nil && !errors.Is(err, errTooLarge) {
		if err := s.markScanned(ctx, x.id, err != nil); err != nil {
			out = append(out, err)
		}
	}
	// Devices found here become pending and raise their alerts now, rather
	// than after the slowest segments in the sweep.
	if err == nil && found > 0 {
		if err := monitoring.New(s.conn).Evaluate(ctx); err != nil {
			out = append(out, err)
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
	if err := ctx.Err(); err != nil { // stopped before this segment started
		return 0, err
	}
	known, err := s.knownAddresses(ctx, seg.id)
	if err != nil {
		return 0, err
	}
	plan := probePlan{seg: seg, hostGateway: defaultGateway(), pinger: icmp, pace: s.pace, portSlots: make(chan struct{}, portSlots), tcpTick: tcpTicker.C,
		due: func(ip string) bool { return s.dueFull(seg, ip, manual) }, portsDue: func(ip string) bool { return s.portsDue(seg, ip) },
		done: func(ip string, ports bool) { s.markFull(seg, ip, ports) }, sendErrors: &sendErrors, firstError: &firstError,
		known: known, silentAll: s.silentDue(seg.id, manual)}
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
