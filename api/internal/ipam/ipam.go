// Package ipam holds the address rules shared by the register, requests and
// reservations: what an Available candidate is, and what is already using an IP.
package ipam

import (
	"context"
	"net/http"
	"net/netip"

	"dfipam/api/internal/netcalc"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
)

// InUse is the SQL condition for a sighting, aliased v, whose address counts as
// in use: it answered in the last 24 hours, or, outside the DHCP pool, in the
// last 30 days. A static device switched off for a weekend or a leave keeps its
// address; inside the pool DHCP hands addresses on, so a day is enough.
const InUse = "(v.last_seen > now() - interval '24 hours' OR (v.is_static AND v.last_seen > now() - interval '30 days'))"

// ErrNoCandidate means every usable address in the segment is taken.
var ErrNoCandidate = httpx.NewError(http.StatusConflict, "NO_CANDIDATE", "This segment has no Available candidate left.")

// Status says what is already true of one address.
type Status struct {
	IP           string  `json:"ip"`
	InSegment    bool    `json:"inSegment"`
	Usable       bool    `json:"usable"`
	InDHCPPool   bool    `json:"inDhcpPool"`
	IsGateway    bool    `json:"isGateway"`
	Registered   bool    `json:"registered"`
	RegisterName string  `json:"registerName"`
	Reserved     bool    `json:"reserved"`
	SeenRecently bool    `json:"seenRecently"`
	SeenMAC      *string `json:"seenMac"`
	Hostname     string  `json:"hostname"`
}

// Available reports whether a person may assign the address without a conflict.
func (s Status) Available() bool {
	return s.InSegment && s.Usable && !s.IsGateway && !s.InDHCPPool && !s.Registered && !s.Reserved
}

// Check describes one address in a segment, in a single query.
func Check(ctx context.Context, conn db.Conn, segmentID string, ip netip.Addr) (Status, error) {
	out := Status{IP: ip.String()}
	var network string
	err := conn.QueryRow(ctx, `SELECT s.network::text, $2::inet <<= s.network,
		s.dhcp_start IS NOT NULL AND $2::inet BETWEEN s.dhcp_start AND s.dhcp_end, s.gateway IS NOT DISTINCT FROM $2::inet,
		i.id IS NOT NULL, COALESCE(i.assigned_name, ''),
		EXISTS (SELECT 1 FROM reservations r WHERE r.segment_id = s.id AND r.status = 'active' AND $2::inet BETWEEN r.start_ip AND r.end_ip),
		v.ip IS NOT NULL, v.mac::text, COALESCE(v.hostname, '')
		FROM segments s
		LEFT JOIN ip_addresses i ON i.ip = $2::inet
		LEFT JOIN LATERAL (SELECT v.ip, v.mac, v.hostname FROM sightings v WHERE v.ip = $2::inet AND `+InUse+`
			ORDER BY v.last_seen DESC LIMIT 1) v ON true
		WHERE s.id = $1::uuid`, segmentID, ip.String()).Scan(&network, &out.InSegment, &out.InDHCPPool, &out.IsGateway,
		&out.Registered, &out.RegisterName, &out.Reserved, &out.SeenRecently, &out.SeenMAC, &out.Hostname)
	if err != nil {
		return out, db.Friendly(err)
	}
	if p, err := netip.ParsePrefix(network); err == nil {
		r := netcalc.UsableRange(p)
		out.Usable = !ip.Less(r.First) && !r.Last.Less(ip)
	}
	return out, nil
}

// Candidate returns the lowest Available candidate: usable, not the gateway,
// outside the DHCP pool, not registered, not reserved and not seen recently.
func Candidate(ctx context.Context, conn db.Conn, segmentID string) (string, error) {
	free, err := Available(ctx, conn, segmentID)
	if err != nil {
		return "", err
	}
	if len(free) == 0 {
		return "", ErrNoCandidate
	}
	return free[0].First.String(), nil
}

// Available lists every run of Available candidates in a segment, lowest first.
// It never walks the range: it subtracts the sorted used ranges from the usable one.
func Available(ctx context.Context, conn db.Conn, segmentID string) ([]netcalc.Range, error) {
	var network string
	if err := conn.QueryRow(ctx, "SELECT network::text FROM segments WHERE id = $1::uuid", segmentID).Scan(&network); err != nil {
		return nil, db.Friendly(err)
	}
	prefix, err := netip.ParsePrefix(network)
	if err != nil {
		return nil, err
	}
	rows, err := conn.Query(ctx, `
		SELECT host(ip), host(ip) FROM ip_addresses WHERE segment_id = $1::uuid
		UNION ALL SELECT host(start_ip), host(end_ip) FROM reservations WHERE segment_id = $1::uuid AND status = 'active'
		UNION ALL SELECT host(dhcp_start), host(dhcp_end) FROM segments WHERE id = $1::uuid AND dhcp_start IS NOT NULL
		UNION ALL SELECT host(gateway), host(gateway) FROM segments WHERE id = $1::uuid AND gateway IS NOT NULL
		UNION ALL SELECT DISTINCT host(v.ip), host(v.ip) FROM sightings v WHERE v.segment_id = $1::uuid AND (`+InUse+` OR v.is_gateway)`,
		segmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var used []netcalc.Range
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		first, e1 := netip.ParseAddr(a)
		last, e2 := netip.ParseAddr(b)
		if e1 == nil && e2 == nil {
			used = append(used, netcalc.Range{First: first, Last: last})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var out []netcalc.Range
	for _, gap := range netcalc.Gaps(netcalc.UsableRange(prefix), used) {
		if gap.First.IsValid() {
			out = append(out, gap)
		}
	}
	return out, nil
}

// RangeSize is the number of addresses from first to last inclusive, for IPv4.
func RangeSize(first, last netip.Addr) int64 {
	return int64(toUint(last)) - int64(toUint(first)) + 1
}

func toUint(a netip.Addr) uint32 {
	b := a.As4()
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}
