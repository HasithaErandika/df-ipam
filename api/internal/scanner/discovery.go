package scanner

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"dfipam/api/internal/modules/monitoring"
	"dfipam/api/internal/netcalc"
)

const maxSweepHosts = 4096

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
	}
	if err := monitoring.New(s.conn).Evaluate(ctx); err != nil {
		scanErrors = append(scanErrors, err)
	}
	// Evidence is kept for 90 days after a device was last seen. Phones that
	// change their private MAC would otherwise leave rows behind forever.
	if _, err := s.conn.Exec(ctx, `DELETE FROM sightings WHERE last_seen < now() - interval '90 days'`); err != nil {
		scanErrors = append(scanErrors, err)
	}
	return errors.Join(scanErrors...)
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
		return nil, fmt.Errorf("network has more than %d scannable addresses; narrow the segment", maxSweepHosts)
	}
	hosts := make([]netip.Addr, 0, int(span.Size().Int64()))
	for ip := span.First; ip.IsValid() && !span.Last.Less(ip); ip = ip.Next() {
		hosts = append(hosts, ip)
	}
	return hosts, nil
}

// scanSegment sweeps one segment. A manual scan identifies every host in full;
// a scheduled one only those not identified in full lately (see fullEvery).
func (s *Scanner) scanSegment(ctx context.Context, seg segment, manual bool) (int, error) {
	hosts, err := scanHosts(seg.network)
	if err != nil {
		return 0, err
	}
	plan := probePlan{seg: seg, hostGateway: defaultGateway(), portSlots: make(chan struct{}, portSlots),
		full: func(ip string) bool { return s.dueFull(seg, ip, manual) }}
	results := make(chan sighting, 64)
	jobs := make(chan netip.Addr)
	ticker := time.NewTicker(10 * time.Millisecond) // At most 100 probes per second.
	defer ticker.Stop()
	var workers sync.WaitGroup
	for range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for ip := range jobs {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
				if found, ok := identify(ctx, plan, ip); ok {
					results <- found
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, ip := range hosts {
			select {
			case <-ctx.Done():
				return
			case jobs <- ip:
			}
		}
	}()
	go func() { workers.Wait(); close(results) }()
	found := make([]sighting, 0)
	for item := range results {
		found = append(found, item)
	}
	// Pinging filled the kernel's neighbour table; where the scanner shares the
	// network with these hosts, it now knows their MACs.
	arp, own := arpTable(), ownMACs()
	for i := range found {
		if mac, ok := arp[found[i].ip]; ok && found[i].mac == "" {
			found[i].mac, found[i].source = mac, "arp"
		} else if mac, ok := own[found[i].ip]; ok && found[i].mac == "" {
			found[i].mac, found[i].source = mac, "arp"
		}
		// The maker named by the MAC's first half made the network card, which
		// is often not the device's maker (an Intel Wi-Fi card in an ASUS laptop).
		found[i].nicVendor = macVendor(found[i].mac)
		if found[i].vendor == "" && found[i].full {
			found[i].vendor = found[i].nicVendor // only when the device had its say
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(found) == 0 {
		return 0, nil
	}
	return len(found), s.saveSightings(ctx, seg.id, found)
}
