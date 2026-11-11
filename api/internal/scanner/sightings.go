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
	nics, makers, models, kinds, systems := make([]string, n), make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	static, private, gateway, ports := make([]bool, n), make([]bool, n), make([]bool, n), make([]string, n)
	for i, v := range values {
		ips[i], macs[i], names[i], sources[i], static[i] = v.ip, strings.ToLower(v.mac), v.hostname, v.source, v.static
		private[i], gateway[i], vendors[i] = isPrivateMAC(macs[i]), v.gateway, v.vendor
		nics[i], makers[i], models[i], kinds[i], systems[i] = v.nicVendor, v.maker, v.model, v.kind, v.os
		ports[i] = portList(v.ports, v.portsChecked)
	}
	// A later scan that can't find a detail keeps the one found before.
	_, err := s.conn.Exec(ctx, `INSERT INTO sightings(segment_id,ip,mac,hostname,source,is_static,is_private_mac,is_gateway,vendor,
			nic_vendor,maker,model,device_kind,os_hint,open_ports,first_seen,last_seen)
		SELECT $1::uuid,ip::inet,NULLIF(mac,'')::macaddr,name,source,static,private,gw,vendor,nic,maker,model,kind,os,COALESCE(NULLIF(ports,'')::integer[],'{}'),now(),now()
		FROM unnest($2::text[],$3::text[],$4::text[],$5::text[],$6::boolean[],$7::boolean[],$8::boolean[],$9::text[],
			$10::text[],$11::text[],$12::text[],$13::text[],$14::text[],$15::text[]) AS v(ip,mac,name,source,static,private,gw,vendor,nic,maker,model,kind,os,ports)
		ON CONFLICT (ip,mac) DO UPDATE SET segment_id=EXCLUDED.segment_id,
			hostname=COALESCE(NULLIF(EXCLUDED.hostname,''),sightings.hostname),
			vendor=COALESCE(NULLIF(EXCLUDED.vendor,''),sightings.vendor),
			nic_vendor=COALESCE(NULLIF(EXCLUDED.nic_vendor,''),sightings.nic_vendor),
			maker=COALESCE(NULLIF(EXCLUDED.maker,''),sightings.maker),
			model=COALESCE(NULLIF(EXCLUDED.model,''),sightings.model),
			device_kind=COALESCE(NULLIF(EXCLUDED.device_kind,''),sightings.device_kind),
			os_hint=COALESCE(NULLIF(EXCLUDED.os_hint,''),sightings.os_hint),
			source=EXCLUDED.source,is_static=EXCLUDED.is_static,is_private_mac=EXCLUDED.is_private_mac,is_gateway=EXCLUDED.is_gateway,last_seen=now()`,
		segmentID, ips, macs, names, sources, static, private, gateway, vendors, nics, makers, models, kinds, systems, ports)
	if err != nil {
		return err
	}
	// Ports are replaced only where they were checked this time; otherwise the
	// last full identification's result stands.
	if _, err := s.conn.Exec(ctx, `UPDATE sightings o SET open_ports = v.ports::integer[]
		FROM unnest($1::text[], $2::text[], $3::text[]) AS v(ip, mac, ports)
		WHERE v.ports <> '' AND o.ip = v.ip::inet AND o.mac IS NOT DISTINCT FROM NULLIF(v.mac, '')::macaddr`, ips, macs, ports); err != nil {
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
