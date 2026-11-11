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
}

type sighting struct {
	ip, mac, hostname, source string
	static                    bool
}

func (s *Scanner) scanAll(ctx context.Context) error {
	rows, err := s.conn.Query(ctx, `SELECT id::text,name,network::text,kind,COALESCE(dhcp_start::text,''),COALESCE(dhcp_end::text,'') FROM segments ORDER BY name`)
	if err != nil {
		return err
	}
	var segments []segment
	for rows.Next() {
		var x segment
		if err := rows.Scan(&x.id, &x.name, &x.network, &x.kind, &x.dhcpStart, &x.dhcpEnd); err != nil {
			rows.Close()
			return err
		}
		segments = append(segments, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	var scanErrors []error
	for _, x := range segments {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.scanSegment(ctx, x); err != nil {
			scanErrors = append(scanErrors, fmt.Errorf("segment %q: %w", x.name, err))
		}
	}
	if err := monitoring.New(s.conn).Evaluate(ctx); err != nil {
		scanErrors = append(scanErrors, err)
	}
	return errors.Join(scanErrors...)
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

func (s *Scanner) scanSegment(ctx context.Context, seg segment) error {
	hosts, err := scanHosts(seg.network)
	if err != nil {
		return err
	}
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
				mac, nbName, _ := netbiosStatus(ctx, ip)
				if name == "" {
					name = nbName
				}
				source := "ping"
				if mac != "" || nbName != "" {
					source = "netbios"
				}
				results <- sighting{ip: ip.String(), mac: mac, hostname: name, source: source,
					static: isStatic(seg, ip)}
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
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(found) == 0 {
		return nil
	}
	return s.saveSightings(ctx, seg.id, found)
}

func isStatic(seg segment, ip netip.Addr) bool {
	if seg.kind == "server" || seg.dhcpStart == "" || seg.dhcpEnd == "" {
		return true
	}
	start, e1 := netip.ParseAddr(seg.dhcpStart)
	end, e2 := netip.ParseAddr(seg.dhcpEnd)
	return e1 != nil || e2 != nil || ip.Less(start) || end.Less(ip)
}

func (s *Scanner) saveSightings(ctx context.Context, segmentID string, values []sighting) error {
	ips, macs, names, sources := make([]string, len(values)), make([]string, len(values)), make([]string, len(values)), make([]string, len(values))
	static := make([]bool, len(values))
	for i, v := range values {
		ips[i], macs[i], names[i], sources[i], static[i] = v.ip, strings.ToLower(v.mac), v.hostname, v.source, v.static
	}
	_, err := s.conn.Exec(ctx, `INSERT INTO sightings(segment_id,ip,mac,hostname,source,is_static,first_seen,last_seen)
		SELECT $1::uuid,ip::inet,NULLIF(mac,'')::macaddr,name,source,static,now(),now()
		FROM unnest($2::text[],$3::text[],$4::text[],$5::text[],$6::boolean[]) AS v(ip,mac,name,source,static)
		ON CONFLICT (ip,mac) DO UPDATE SET segment_id=EXCLUDED.segment_id,hostname=EXCLUDED.hostname,
			source=EXCLUDED.source,is_static=EXCLUDED.is_static,last_seen=now()`, segmentID, ips, macs, names, sources, static)
	return err
}
