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
	ips, macs, names, sources, vendors := make([]string, n), make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	static, private, gateway := make([]bool, n), make([]bool, n), make([]bool, n)
	for i, v := range values {
		ips[i], macs[i], names[i], sources[i], static[i] = v.ip, strings.ToLower(v.mac), v.hostname, v.source, v.static
		private[i], gateway[i], vendors[i] = isPrivateMAC(macs[i]), v.gateway, v.vendor
	}
	// A later scan that can't resolve a name keeps the one found before.
	_, err := s.conn.Exec(ctx, `INSERT INTO sightings(segment_id,ip,mac,hostname,source,is_static,is_private_mac,is_gateway,vendor,first_seen,last_seen)
		SELECT $1::uuid,ip::inet,NULLIF(mac,'')::macaddr,name,source,static,private,gw,vendor,now(),now()
		FROM unnest($2::text[],$3::text[],$4::text[],$5::text[],$6::boolean[],$7::boolean[],$8::boolean[],$9::text[]) AS v(ip,mac,name,source,static,private,gw,vendor)
		ON CONFLICT (ip,mac) DO UPDATE SET segment_id=EXCLUDED.segment_id,
			hostname=COALESCE(NULLIF(EXCLUDED.hostname,''),sightings.hostname),
			vendor=COALESCE(NULLIF(EXCLUDED.vendor,''),sightings.vendor),
			source=EXCLUDED.source,is_static=EXCLUDED.is_static,is_private_mac=EXCLUDED.is_private_mac,is_gateway=EXCLUDED.is_gateway,last_seen=now()`,
		segmentID, ips, macs, names, sources, static, private, gateway, vendors)
	if err != nil {
		return err
	}
	// An address first seen by ping alone, now seen with its MAC, is the same
	// host: drop the MAC-less row so it isn't listed twice.
	_, err = s.conn.Exec(ctx, `DELETE FROM sightings o USING unnest($1::text[], $2::text[]) AS v(ip, mac)
		WHERE v.mac <> '' AND o.mac IS NULL AND o.ip = v.ip::inet`, ips, macs)
	return err
}

// isPrivateMAC reports a locally administered address, as phones and laptops use
// for MAC randomisation on Wi-Fi. They are marked so they don't flood the unknown list.
func isPrivateMAC(mac string) bool {
	hw, err := net.ParseMAC(mac)
	return err == nil && len(hw) > 0 && hw[0]&0x02 != 0
}
