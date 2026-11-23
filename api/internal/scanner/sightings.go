package scanner

import (
	"context"
	"net"
	"net/netip"
	"strings"
)

// isStatic reports an address outside the DHCP pool. A DHCP network with no
// pool set can't tell, so nothing on it counts as static (and raises no
// unregistered-static alerts) until the pool is set.
func isStatic(seg segment, ip netip.Addr) bool {
	if seg.kind != "dhcp" {
		return true
	}
	if seg.dhcpStart == "" || seg.dhcpEnd == "" {
		return false
	}
	start, e1 := netip.ParseAddr(seg.dhcpStart)
	end, e2 := netip.ParseAddr(seg.dhcpEnd)
	return e1 != nil || e2 != nil || ip.Less(start) || end.Less(ip)
}

func (s *Scanner) saveSightings(ctx context.Context, segmentID string, values []sighting) error {
	if err := s.attachMACs(ctx, values); err != nil {
		return err
	}
	n := len(values)
	ips, macs, names, sources, vendors := make([]string, n), make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	nics, makers, models, kinds, systems := make([]string, n), make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	static, private, gateway, ports := make([]bool, n), make([]bool, n), make([]bool, n), make([]string, n)
	full := make([]bool, n)
	groups, wdomains, fqdns, builds := make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	for i, v := range values {
		full[i] = v.full
		groups[i], fqdns[i], builds[i] = v.workgroup, v.windows.fqdn, v.windows.build
		wdomains[i] = v.windows.dnsDomain
		if wdomains[i] == "" {
			wdomains[i] = v.windows.domain
		}
		ips[i], macs[i], names[i], sources[i], static[i] = v.ip, strings.ToLower(v.mac), v.hostname, v.source, v.static
		private[i], gateway[i], vendors[i] = isPrivateMAC(macs[i]), v.gateway, v.vendor
		nics[i], makers[i], models[i], kinds[i], systems[i] = v.nicVendor, v.maker, v.model, v.kind, v.os
		ports[i] = portList(v.ports, v.portsChecked)
	}
	// A later scan that can't find a detail keeps the one found before.
	_, err := s.conn.Exec(ctx, `INSERT INTO sightings(segment_id,ip,mac,hostname,source,is_static,is_private_mac,is_gateway,vendor,
			nic_vendor,maker,model,device_kind,os_hint,open_ports,identified_at,first_seen,last_seen,workgroup,windows_domain,windows_fqdn,os_version)
		SELECT $1::uuid,ip::inet,NULLIF(mac,'')::macaddr,name,source,static,private,gw,vendor,nic,maker,model,kind,os,COALESCE(NULLIF(ports,'')::integer[],'{}'),
			CASE WHEN identified THEN now() END,now(),now(),grp,wdomain,fqdn,build
		FROM unnest($2::text[],$3::text[],$4::text[],$5::text[],$6::boolean[],$7::boolean[],$8::boolean[],$9::text[],
			$10::text[],$11::text[],$12::text[],$13::text[],$14::text[],$15::text[],$16::boolean[],$17::text[],$18::text[],$19::text[],$20::text[])
			AS v(ip,mac,name,source,static,private,gw,vendor,nic,maker,model,kind,os,ports,identified,grp,wdomain,fqdn,build)
		-- Rows are locked in (ip, mac) order, the same order as the FortiGate
		-- reader, so the two can't deadlock on the same sightings.
		ORDER BY ip::inet, mac
		ON CONFLICT (ip,mac) DO UPDATE SET segment_id=EXCLUDED.segment_id,
			hostname=COALESCE(NULLIF(EXCLUDED.hostname,''),sightings.hostname),
			vendor=COALESCE(NULLIF(EXCLUDED.vendor,''),sightings.vendor),
			nic_vendor=COALESCE(NULLIF(EXCLUDED.nic_vendor,''),sightings.nic_vendor),
			maker=COALESCE(NULLIF(EXCLUDED.maker,''),sightings.maker),
			model=COALESCE(NULLIF(EXCLUDED.model,''),sightings.model),
			device_kind=COALESCE(NULLIF(EXCLUDED.device_kind,''),sightings.device_kind),
			os_hint=COALESCE(NULLIF(EXCLUDED.os_hint,''),sightings.os_hint),
			identified_at=COALESCE(EXCLUDED.identified_at,sightings.identified_at),
			workgroup=COALESCE(NULLIF(EXCLUDED.workgroup,''),sightings.workgroup),
			windows_domain=COALESCE(NULLIF(EXCLUDED.windows_domain,''),sightings.windows_domain),
			windows_fqdn=COALESCE(NULLIF(EXCLUDED.windows_fqdn,''),sightings.windows_fqdn),
			os_version=COALESCE(NULLIF(EXCLUDED.os_version,''),sightings.os_version),
			source=EXCLUDED.source,is_static=EXCLUDED.is_static,is_private_mac=EXCLUDED.is_private_mac,is_gateway=EXCLUDED.is_gateway,last_seen=now()`,
		segmentID, ips, macs, names, sources, static, private, gateway, vendors, nics, makers, models, kinds, systems, ports, full, groups, wdomains, fqdns, builds)
	if err != nil {
		return err
	}
	// Ports are replaced only where they were checked this time; otherwise the
	// last full identification's result stands.
	if _, err := s.conn.Exec(ctx, `UPDATE sightings o SET open_ports = v.ports::integer[], ports_checked_at = now()
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

// attachMACs gives a sighting with no MAC the MAC last seen at its address
// within a day. NetBIOS sometimes misses its reply on a routed network; the
// host is the same one, so it shouldn't be listed again without its MAC. Only
// outside the DHCP pool: inside it the address may belong to another device by
// now, which would then be credited to the old one.
func (s *Scanner) attachMACs(ctx context.Context, values []sighting) error {
	var bare []string
	for _, v := range values {
		if v.mac == "" && v.static {
			bare = append(bare, v.ip)
		}
	}
	if len(bare) == 0 {
		return nil
	}
	rows, err := s.conn.Query(ctx, `SELECT DISTINCT ON (ip) host(ip), mac::text FROM sightings
		WHERE ip = ANY($1::inet[]) AND mac IS NOT NULL AND last_seen > now() - interval '1 day' ORDER BY ip, last_seen DESC`, bare)
	if err != nil {
		return err
	}
	defer rows.Close()
	known := map[string]string{}
	for rows.Next() {
		var ip, mac string
		if err := rows.Scan(&ip, &mac); err != nil {
			return err
		}
		known[ip] = mac
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for i := range values {
		if mac, ok := known[values[i].ip]; ok && values[i].mac == "" && values[i].static {
			values[i].mac = mac
			values[i].nicVendor = macVendor(mac)
		}
	}
	return nil
}

// isPrivateMAC reports a locally administered address, as phones and laptops use
// for MAC randomisation on Wi-Fi. They are marked so they don't flood the unknown list.
func isPrivateMAC(mac string) bool {
	hw, err := net.ParseMAC(mac)
	return err == nil && len(hw) > 0 && hw[0]&0x02 != 0
}
