package scanner

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
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
}

type sighting struct {
	ip, mac, hostname, source, vendor string
	static, gateway                   bool
}

func (s *Scanner) loadSegments(ctx context.Context, only string) ([]segment, error) {
	rows, err := s.conn.Query(ctx, `SELECT id::text, name, network::text, kind, COALESCE(host(dhcp_start), ''), COALESCE(host(dhcp_end), ''), COALESCE(host(gateway), '')
		FROM segments WHERE kind <> 'cloud' AND ($1 = '' OR id::text = $1) ORDER BY kind, floor NULLS LAST, name`, only)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []segment
	for rows.Next() {
		var x segment
		if err := rows.Scan(&x.id, &x.name, &x.network, &x.kind, &x.dhcpStart, &x.dhcpEnd, &x.gateway); err != nil {
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
		found, err := s.scanSegment(ctx, x)
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

func (s *Scanner) scanSegment(ctx context.Context, seg segment) (int, error) {
	hosts, err := scanHosts(seg.network)
	if err != nil {
		return 0, err
	}
	hostGateway := defaultGateway()
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
				alive, err := ping(ctx, ip, 700*time.Millisecond)
				if err != nil || !alive {
					continue
				}
				name, _ := reverseName(ctx, ip)
				gateway := name == "_gateway" || ip.String() == seg.gateway || ip.String() == hostGateway
				if strings.HasPrefix(name, "_") || name == "localhost" {
					name = "" // synthetic names from the local resolver, such as _gateway
				}
				mac, nbName, _ := netbiosStatus(ctx, ip)
				if name == "" {
					name = nbName
				}
				if name == "" {
					name = mdnsName(ctx, ip)
				}
				upnp := ssdpDescribe(ctx, ip)
				if name == "" {
					name = upnp.Name
				}
				source := "ping"
				if mac != "" || nbName != "" {
					source = "netbios"
				}
				results <- sighting{ip: ip.String(), mac: mac, hostname: name, source: source,
					static: isStatic(seg, ip), gateway: gateway, vendor: upnp.Describe()}
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
		if found[i].vendor == "" && !isPrivateMAC(found[i].mac) {
			found[i].vendor = macVendor(found[i].mac) // the maker named by the MAC's first half
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
