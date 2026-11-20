package reports

import "strings"

// Report is one downloadable report from docs/product/screens.md#reports. Every column
// is selected as text, so any report renders the same way. Queries are constants;
// only the date range and environment are bound, as $1, $2 and $3.
type Report struct {
	Key         string   `json:"key"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Dated       bool     `json:"dated"`       // uses the date range
	ByEnv       bool     `json:"environment"` // can filter by environment
	Ahead       bool     `json:"ahead"`       // the date range looks forward, as for renewals
	Columns     []string `json:"columns"`
	// Needs lists permissions a person must also hold, beyond reports:export.
	Needs []string `json:"-"`
	sql   string
}

// $1 from, $2 to (exclusive end, as dates), $3 environment ID or ”.
var catalogue = []Report{
	{Key: "segment-usage", Title: "Segment usage", Description: "Per DHCP, server, cloud and VPN network: registered, reserved, seen in use and DHCP pool use.",
		Columns: []string{"Segment", "Type", "Network", "VLAN", "Assigned", "Reserved", "Blocked", "Held by reservations", "Seen (24h)", "Pool size", "Pool in use"},
		sql: `SELECT s.name, s.kind, s.network::text, COALESCE(s.vlan::text, ''),
			(SELECT count(*) FROM ip_addresses i WHERE i.segment_id = s.id AND i.status = 'assigned')::text,
			(SELECT count(*) FROM ip_addresses i WHERE i.segment_id = s.id AND i.status = 'reserved')::text,
			(SELECT count(*) FROM ip_addresses i WHERE i.segment_id = s.id AND i.status = 'blocked')::text,
			COALESCE((SELECT sum(r.end_ip - r.start_ip + 1) FROM reservations r WHERE r.segment_id = s.id AND r.status = 'active'), 0)::text,
			(SELECT count(DISTINCT v.ip) FROM sightings v WHERE v.segment_id = s.id AND v.last_seen > now() - interval '24 hours')::text,
			COALESCE((s.dhcp_end - s.dhcp_start + 1)::text, ''),
			CASE WHEN s.dhcp_start IS NULL THEN '' ELSE (SELECT count(DISTINCT v.ip) FROM sightings v WHERE v.segment_id = s.id
				AND v.last_seen > now() - interval '24 hours' AND v.ip BETWEEN s.dhcp_start AND s.dhcp_end)::text END
			FROM segments s WHERE $1::date IS NOT NULL AND $2::date IS NOT NULL AND $3::text IS NOT NULL ORDER BY s.kind, s.floor NULLS LAST, s.name`},
	{Key: "servers-by-environment", Title: "Servers by environment", Description: "Every monitored or server-segment entry with its environment and current status.", ByEnv: true,
		Columns: []string{"Environment", "IP", "Name", "Segment", "Status", "Since", "Critical", "Monitor", "Owner"},
		sql: `SELECT COALESCE(e.name, 'None'), host(i.ip), i.assigned_name, s.name, i.health, COALESCE(to_char(i.health_changed_at, 'YYYY-MM-DD HH24:MI'), ''),
			CASE WHEN i.critical THEN 'Yes' ELSE 'No' END, CASE WHEN i.monitor THEN 'Yes' ELSE 'No' END, i.owner
			FROM ip_addresses i JOIN segments s ON s.id = i.segment_id LEFT JOIN environments e ON e.id = COALESCE(i.environment_id, s.default_environment_id)
			WHERE (s.kind IN ('server', 'cloud') OR i.monitor) AND ($3::text = '' OR e.id::text = $3::text) AND $1::date IS NOT NULL AND $2::date IS NOT NULL
			ORDER BY e.sort_order NULLS LAST, i.ip`},
	{Key: "ip-register", Title: "IP register", Description: "Every registered IP with type, owner and status.", ByEnv: true,
		Columns: []string{"IP", "Segment", "Status", "Type", "Environment", "Assigned to", "Platform", "Owner", "Department", "Expected MAC"},
		sql: `SELECT host(i.ip), s.name, i.status, i.ip_type, COALESCE(e.name, ''), i.assigned_name, i.platform, i.owner, i.department, COALESCE(i.expected_mac::text, '')
			FROM ip_addresses i JOIN segments s ON s.id = i.segment_id LEFT JOIN environments e ON e.id = i.environment_id
			WHERE ($3::text = '' OR e.id::text = $3::text) AND $1::date IS NOT NULL AND $2::date IS NOT NULL ORDER BY i.ip`},
	{Key: "unknown-devices", Title: "Unknown and pending devices", Description: "Devices seen but not approved, with first and last seen.",
		Columns: []string{"MAC", "Status", "Name", "Last IP", "Segment", "Hostname", "Domain or workgroup", "In Intune", "First seen", "Last seen"},
		sql: `SELECT d.mac::text, d.status, d.name, COALESCE(host(v.ip), ''), COALESCE(s.name, ''), COALESCE(v.hostname, ''),
			COALESCE(NULLIF(v.windows_domain, ''), v.workgroup, ''), CASE WHEN v.intune_seen_at IS NULL THEN 'No' ELSE 'Yes' END,
			COALESCE(to_char(d.first_seen, 'YYYY-MM-DD HH24:MI'), ''), COALESCE(to_char(d.last_seen, 'YYYY-MM-DD HH24:MI'), '')
			FROM devices d LEFT JOIN LATERAL (SELECT ip, segment_id, hostname, windows_domain, workgroup, intune_seen_at FROM sightings
				WHERE mac = d.mac ORDER BY last_seen DESC LIMIT 1) v ON true
			LEFT JOIN segments s ON s.id = v.segment_id
			WHERE d.status <> 'approved' AND $1::date IS NOT NULL AND $2::date IS NOT NULL AND $3::text IS NOT NULL ORDER BY d.last_seen DESC NULLS LAST`},
	{Key: "device-users", Title: "Device users and management", Description: "Devices seen in the last 30 days with their user (Intune, Entra ID or VPN), domain, Windows version and Intune record.",
		Columns: []string{"IP", "Segment", "Name", "MAC", "User", "User from", "Domain or workgroup", "Windows version", "Serial", "Maker and model",
			"Operating system", "Compliance", "Encrypted", "Last Intune check-in", "Last seen"},
		sql: `SELECT host(v.ip), COALESCE(s.name, ''), v.hostname, COALESCE(v.mac::text, ''), COALESCE(NULLIF(v.intune_user, ''), NULLIF(v.entra_user, ''), v.vpn_user),
			CASE WHEN v.intune_user <> '' THEN 'Intune' WHEN v.entra_user <> '' THEN 'Entra ID' WHEN v.vpn_user <> '' THEN 'VPN' ELSE '' END,
			COALESCE(NULLIF(v.windows_domain, ''), v.workgroup), v.os_version, v.intune_serial, trim(v.intune_maker || ' ' || v.intune_model), v.intune_os,
			v.intune_compliance, CASE v.intune_encrypted WHEN true THEN 'Yes' WHEN false THEN 'No' ELSE '' END,
			COALESCE(to_char(v.intune_last_sync, 'YYYY-MM-DD HH24:MI'), ''), to_char(v.last_seen, 'YYYY-MM-DD HH24:MI')
			FROM sightings v LEFT JOIN segments s ON s.id = v.segment_id
			WHERE v.last_seen > now() - interval '30 days' AND $1::date IS NOT NULL AND $2::date IS NOT NULL AND $3::text IS NOT NULL
			ORDER BY v.ip, v.last_seen DESC`},
	{Key: "vpn-sessions", Title: "VPN sessions", Description: "The latest VPN session on each tunnel address seen in the date range: user, VPN kind and where they connected from. Earlier sessions on the same address are replaced.", Dated: true,
		Columns: []string{"User", "Tunnel IP", "VPN", "Connected from", "Segment", "First seen", "Last seen"},
		sql: `SELECT v.vpn_user, host(v.ip), CASE v.vpn_kind WHEN 'ssl' THEN 'SSL VPN' WHEN 'ipsec' THEN 'IPsec ' || v.vpn_tunnel ELSE '' END, v.vpn_remote,
			COALESCE(s.name, ''), to_char(v.first_seen, 'YYYY-MM-DD HH24:MI'), to_char(v.last_seen, 'YYYY-MM-DD HH24:MI')
			FROM sightings v LEFT JOIN segments s ON s.id = v.segment_id
			WHERE v.source = 'vpn' AND v.last_seen >= $1::date AND v.last_seen < $2::date + 1 AND $3::text IS NOT NULL ORDER BY v.last_seen DESC`},
	{Key: "static-on-floors", Title: "Static IPs in DHCP networks", Description: "Addresses in use outside the DHCP pool in DHCP networks (floors and Wi-Fi), registered or not.",
		Columns: []string{"IP", "Segment", "MAC", "Hostname", "Registered", "Assigned to", "First seen", "Last seen"},
		sql: `SELECT DISTINCT ON (v.ip) host(v.ip), s.name, COALESCE(v.mac::text, ''), v.hostname, CASE WHEN i.id IS NULL THEN 'No' ELSE 'Yes' END,
			COALESCE(i.assigned_name, ''), to_char(v.first_seen, 'YYYY-MM-DD HH24:MI'), to_char(v.last_seen, 'YYYY-MM-DD HH24:MI')
			FROM sightings v JOIN segments s ON s.id = v.segment_id AND s.kind = 'dhcp' LEFT JOIN ip_addresses i ON i.ip = v.ip
			WHERE v.is_static AND v.last_seen >= $1::date AND v.last_seen < $2::date + 1 AND $3::text IS NOT NULL ORDER BY v.ip, v.last_seen DESC`, Dated: true},
	{Key: "server-availability", Title: "Server availability", Description: "Outages per monitored server in the date range, from Server down alerts.", Dated: true, ByEnv: true,
		Columns: []string{"Environment", "IP", "Name", "Outages", "Downtime (minutes)", "Availability %"},
		sql: `WITH span AS (SELECT $1::date::timestamptz AS a, LEAST(($2::date + 1)::timestamptz, now()) AS b)
			SELECT COALESCE(e.name, 'None'), host(i.ip), i.assigned_name, count(al.id)::text,
				COALESCE(round(sum(EXTRACT(EPOCH FROM LEAST(COALESCE(al.closed_at, now()), span.b) - GREATEST(al.opened_at, span.a)) / 60))::text, '0'),
				CASE WHEN span.b <= span.a THEN '' ELSE to_char(100 - 100 * COALESCE(sum(EXTRACT(EPOCH FROM LEAST(COALESCE(al.closed_at, now()), span.b)
					- GREATEST(al.opened_at, span.a))), 0) / EXTRACT(EPOCH FROM span.b - span.a), 'FM990.00') END
			FROM ip_addresses i CROSS JOIN span JOIN segments s ON s.id = i.segment_id
			LEFT JOIN environments e ON e.id = COALESCE(i.environment_id, s.default_environment_id)
			LEFT JOIN alerts al ON al.ip_address_id = i.id AND al.alert_type = 'server_down' AND al.opened_at < span.b AND COALESCE(al.closed_at, now()) > span.a
			WHERE i.monitor AND ($3::text = '' OR e.id::text = $3::text)
			GROUP BY e.name, e.sort_order, i.ip, i.assigned_name, span.a, span.b ORDER BY e.sort_order NULLS LAST, i.ip`},
	{Key: "alert-history", Title: "Alert history", Description: "Alerts raised in the date range, with acknowledgement and closing times.", Dated: true,
		Columns: []string{"Opened", "Severity", "Type", "Status", "IP", "MAC", "Segment", "Message", "Acknowledged by", "Closed"},
		sql: `SELECT to_char(a.opened_at, 'YYYY-MM-DD HH24:MI'), a.severity, a.alert_type, a.status, COALESCE(host(a.ip), ''), COALESCE(a.mac::text, ''),
			COALESCE(s.name, ''), a.message, COALESCE(u.email, ''), COALESCE(to_char(a.closed_at, 'YYYY-MM-DD HH24:MI'), '')
			FROM alerts a LEFT JOIN segments s ON s.id = a.segment_id LEFT JOIN users u ON u.id = a.acknowledged_by
			WHERE a.opened_at >= $1::date AND a.opened_at < $2::date + 1 AND $3::text IS NOT NULL ORDER BY a.opened_at DESC`},
	{Key: "requests-reservations", Title: "Requests and reservations", Description: "IP requests raised and reservations made in the date range, with their state.", Dated: true,
		Columns: []string{"Kind", "Created", "Status", "Name or reason", "Segment", "IP or range", "Requester or owner", "Decided or expires"},
		sql: `SELECT 'Request', to_char(r.created_at, 'YYYY-MM-DD'), r.status, r.name, s.name, COALESCE(host(a.ip), host(r.requested_ip), ''), u.email,
			COALESCE(to_char(r.decided_at, 'YYYY-MM-DD'), '')
			FROM ip_requests r JOIN segments s ON s.id = r.segment_id JOIN users u ON u.id = r.requester_id LEFT JOIN ip_addresses a ON a.id = r.assigned_ip_id
			WHERE r.created_at >= $1::date AND r.created_at < $2::date + 1 AND $3::text IS NOT NULL
			UNION ALL
			SELECT 'Reservation', to_char(h.created_at, 'YYYY-MM-DD'), h.status, h.reason, s.name,
				host(h.start_ip) || CASE WHEN h.start_ip = h.end_ip THEN '' ELSE ' – ' || host(h.end_ip) END, h.owner_name,
				COALESCE(to_char(h.expires_at - interval '1 second', 'YYYY-MM-DD'), 'No expiry')
			FROM reservations h JOIN segments s ON s.id = h.segment_id
			WHERE h.created_at >= $1::date AND h.created_at < $2::date + 1 ORDER BY 2 DESC`},
	{Key: "audit-log", Title: "Audit log", Description: "Every change in the date range: who, what and when.", Dated: true,
		Columns: []string{"When", "Who", "Action", "Record", "Record ID", "Source IP"},
		sql: `SELECT to_char(a.occurred_at, 'YYYY-MM-DD HH24:MI:SS'), COALESCE(u.email, a.actor_type), a.action, a.object_type, COALESCE(a.object_id, ''),
			COALESCE(host(a.source_ip), '')
			FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id
			WHERE a.occurred_at >= $1::date AND a.occurred_at < $2::date + 1 AND $3::text IS NOT NULL ORDER BY a.occurred_at DESC`},
}

// all is every report, the Network Links ones last.
func all() []Report { return append(append([]Report{}, catalogue...), linkReports...) }

func find(key string) (Report, bool) {
	for _, r := range all() {
		if r.Key == key {
			return r, true
		}
	}
	return Report{}, false
}

func sprintf(format, v string) string { return strings.ReplaceAll(format, "%s", v) }
