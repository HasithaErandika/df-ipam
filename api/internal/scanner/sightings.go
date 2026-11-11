package scanner

import (
	"context"
	"net"
	"net/netip"
	"strings"
)

func isStatic(seg segment, ip netip.Addr) bool {
	if seg.kind == "server" || seg.dhcpStart == "" || seg.dhcpEnd == "" {
		return true
	}
	start, e1 := netip.ParseAddr(seg.dhcpStart)
	end, e2 := netip.ParseAddr(seg.dhcpEnd)
	return e1 != nil || e2 != nil || ip.Less(start) || end.Less(ip)
}

func (s *Scanner) saveSightings(ctx context.Context, segmentID string, values []sighting) error {
	n := len(values)
	ips, macs, names, sources := make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	static, private := make([]bool, n), make([]bool, n)
	for i, v := range values {
		ips[i], macs[i], names[i], sources[i], static[i] = v.ip, strings.ToLower(v.mac), v.hostname, v.source, v.static
		private[i] = isPrivateMAC(macs[i])
	}
	// A later scan that can't resolve a name keeps the one found before.
	_, err := s.conn.Exec(ctx, `INSERT INTO sightings(segment_id,ip,mac,hostname,source,is_static,is_private_mac,first_seen,last_seen)
		SELECT $1::uuid,ip::inet,NULLIF(mac,'')::macaddr,name,source,static,private,now(),now()
		FROM unnest($2::text[],$3::text[],$4::text[],$5::text[],$6::boolean[],$7::boolean[]) AS v(ip,mac,name,source,static,private)
		ON CONFLICT (ip,mac) DO UPDATE SET segment_id=EXCLUDED.segment_id,
			hostname=COALESCE(NULLIF(EXCLUDED.hostname,''),sightings.hostname),
			source=EXCLUDED.source,is_static=EXCLUDED.is_static,is_private_mac=EXCLUDED.is_private_mac,last_seen=now()`,
		segmentID, ips, macs, names, sources, static, private)
	return err
}

// isPrivateMAC reports a locally administered address, as phones and laptops use
// for MAC randomisation on Wi-Fi. They are marked so they don't flood the unknown list.
func isPrivateMAC(mac string) bool {
	hw, err := net.ParseMAC(mac)
	return err == nil && len(hw) > 0 && hw[0]&0x02 != 0
}
