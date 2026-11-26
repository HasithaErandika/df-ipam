package fortigate

import (
	"context"
	"net"
	"strings"

	"dfipam/api/internal/platform/db"
)

// save records entries as sightings. ARP and DHCP evidence is kept only inside a
// known segment, so WAN neighbours don't appear; VPN clients are kept regardless,
// and land in a VPN segment when one covers the FortiGate's client pool.
// A DHCP lease is never static; an ARP entry is static outside the floor's pool.
//
// A lease lasts hours or days after a device leaves, so it isn't proof that the
// device is there: only a lease the firewall's ARP table confirms counts as seen
// now. Other leases only add their name and vendor class to evidence already held.
func save(ctx context.Context, conn db.Conn, entries []Entry) error {
	present, enrich := splitEvidence(entries)
	if err := savePresent(ctx, conn, present); err != nil {
		return err
	}
	return saveDetails(ctx, conn, enrich)
}

func normalMAC(mac string) string {
	if hw, err := net.ParseMAC(strings.ToLower(mac)); err == nil {
		return hw.String()
	}
	return ""
}

// splitEvidence merges entries per IP and MAC, preferring a lease's details, and
// sets aside leases the ARP table doesn't confirm.
func splitEvidence(entries []Entry) (present, enrich []Entry) {
	inARP := markARP(entries)
	merged := map[string]Entry{}
	var order []string
	for _, e := range entries {
		key := e.IP + "|" + e.MAC
		if e.Source == "dhcp" && !inARP[key] {
			if e.MAC != "" {
				enrich = append(enrich, e)
			}
			continue
		}
		old, ok := merged[key]
		if ok && old.Source == "dhcp" {
			continue // a lease says more than the ARP entry for the same pair
		}
		if !ok {
			order = append(order, key)
		}
		merged[key] = e
	}
	for _, k := range order {
		present = append(present, merged[k])
	}
	return present, enrich
}

// markARP normalises every MAC in place and returns the IP|MAC pairs the ARP
// table holds.
func markARP(entries []Entry) map[string]bool {
	inARP := map[string]bool{}
	for i := range entries {
		entries[i].MAC = normalMAC(entries[i].MAC)
		if entries[i].Source == "arp" {
			inARP[entries[i].IP+"|"+entries[i].MAC] = true
		}
	}
	return inARP
}

func savePresent(ctx context.Context, conn db.Conn, entries []Entry) error {
	if len(entries) == 0 {
		return nil
	}
	n := len(entries)
	ips, macs, names, vendors, sources := make([]string, n), make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	users, kinds, tunnels, remotes := make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	private := make([]bool, n)
	for i, e := range entries {
		ips[i], macs[i], names[i], vendors[i], sources[i] = e.IP, e.MAC, e.Hostname, e.Vendor, e.Source
		users[i], kinds[i], tunnels[i], remotes[i] = e.User, e.VPNKind, e.Tunnel, e.Remote
		hw, _ := net.ParseMAC(e.MAC)
		private[i] = len(hw) > 0 && hw[0]&0x02 != 0
	}
	// A VPN row's user, kind, tunnel and origin are replaced, not kept: the
	// address may now belong to another person's session.
	_, err := conn.Exec(ctx, `INSERT INTO sightings (segment_id, ip, mac, hostname, vendor, source, is_static, is_private_mac, first_seen, last_seen,
			vpn_user, vpn_kind, vpn_tunnel, vpn_remote)
		SELECT s.id, x.ip::inet, NULLIF(x.mac, '')::macaddr, x.name, x.vendor, x.source,
			CASE WHEN x.source IN ('dhcp', 'vpn') THEN false
			     WHEN s.kind <> 'dhcp' THEN true
			     WHEN s.dhcp_start IS NULL THEN false
			     ELSE NOT (x.ip::inet BETWEEN s.dhcp_start AND s.dhcp_end) END,
			x.private, now(), now(), x.usr, x.kind, x.tunnel, x.remote
		FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::boolean[], $7::text[], $8::text[], $9::text[], $10::text[])
			AS x(ip, mac, name, vendor, source, private, usr, kind, tunnel, remote)
		LEFT JOIN segments s ON x.ip::inet <<= s.network
		WHERE s.id IS NOT NULL OR x.source = 'vpn'
		-- Locked in (ip, mac) order, as the scanner does, so the two can't deadlock.
		ORDER BY x.ip::inet, x.mac
		ON CONFLICT (ip, mac) DO UPDATE SET segment_id = EXCLUDED.segment_id,
			hostname = COALESCE(NULLIF(EXCLUDED.hostname, ''), sightings.hostname),
			vendor = COALESCE(NULLIF(EXCLUDED.vendor, ''), sightings.vendor),
			source = EXCLUDED.source, is_static = EXCLUDED.is_static, is_private_mac = EXCLUDED.is_private_mac, last_seen = now(),
			vpn_user = EXCLUDED.vpn_user, vpn_kind = EXCLUDED.vpn_kind, vpn_tunnel = EXCLUDED.vpn_tunnel, vpn_remote = EXCLUDED.vpn_remote`,
		ips, macs, names, vendors, sources, private, users, kinds, tunnels, remotes)
	return err
}

// saveDetails adds lease names and vendor classes to sightings already held,
// without marking them as seen.
func saveDetails(ctx context.Context, conn db.Conn, leases []Entry) error {
	if len(leases) == 0 {
		return nil
	}
	ips, macs, names, vendors := make([]string, len(leases)), make([]string, len(leases)), make([]string, len(leases)), make([]string, len(leases))
	for i, e := range leases {
		ips[i], macs[i], names[i], vendors[i] = e.IP, e.MAC, e.Hostname, e.Vendor
	}
	_, err := conn.Exec(ctx, `UPDATE sightings o SET hostname = COALESCE(NULLIF(v.name, ''), o.hostname), vendor = COALESCE(NULLIF(v.vendor, ''), o.vendor)
		FROM unnest($1::text[], $2::text[], $3::text[], $4::text[]) AS v(ip, mac, name, vendor)
		WHERE o.ip = v.ip::inet AND o.mac = v.mac::macaddr`, ips, macs, names, vendors)
	return err
}
