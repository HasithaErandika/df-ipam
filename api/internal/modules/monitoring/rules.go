package monitoring

import "dfipam/api/internal/notify"

// delivery is how an alert of this severity is emailed.
func delivery(severity string) notify.Delivery {
	switch severity {
	case "critical":
		return notify.Now
	case "high":
		return notify.Digest
	}
	return notify.AppOnly
}

// candidatesSQL lists every alert cause that is true right now (docs/product/alerts.md).
// "Recent" is seen_window(): two scan intervals, so alerts don't close between two slow scans.
// An address in a DHCP pool is a conflict only when two MACs answered within five
// minutes (one scan): DHCP often gives an address to another device within the window.
// Private (randomised) MACs never count as unknown devices. A segment's gateway
// (set on the segment, or detected) is infrastructure: it never counts as an
// unregistered static or unknown server address.
const candidatesSQL = `
WITH recent AS (
 SELECT si.*, sg.kind, sg.name AS segment_name, sg.alert_private_macs,
        max(si.last_seen) FILTER (WHERE si.mac IS NOT NULL) OVER (PARTITION BY si.ip) AS ip_latest,
        (si.is_gateway OR si.ip IS NOT DISTINCT FROM sg.gateway) AS gateway_ip, COALESCE(env.name, '') AS environment_name,
        COALESCE(env.default_severity, 'medium') AS environment_severity, dev.id AS device_id, dev.status AS device_status,
        ipr.id AS register_id, ipr.status AS register_status, ipr.expected_mac, ipr.critical, ipr.assigned_name
 FROM sightings si JOIN segments sg ON sg.id = si.segment_id
 LEFT JOIN devices dev ON dev.mac = si.mac
 LEFT JOIN ip_addresses ipr ON ipr.ip = si.ip
 LEFT JOIN environments env ON env.id = COALESCE(ipr.environment_id, sg.default_environment_id)
 WHERE si.last_seen > now() - (SELECT seen_window())
), rules AS (
 SELECT CASE WHEN r.device_status = 'rejected' THEN 'rejected_device' ELSE 'unknown_device' END AS typ,
        CASE WHEN r.device_status = 'rejected' OR r.kind IN ('server', 'cloud') THEN 'high' ELSE 'medium' END AS severity,
        r.segment_id, r.register_id AS ipid, r.device_id, host(r.ip) AS ip, r.mac::text AS mac,
        CASE WHEN r.device_status = 'rejected' THEN 'Rejected device ' ELSE 'Unapproved device ' END || r.mac || ' is active at ' || host(r.ip) || ' on ' || r.segment_name AS message
 FROM recent r WHERE r.mac IS NOT NULL AND NOT r.is_private_mac AND r.device_status IS DISTINCT FROM 'approved'
 UNION ALL
 SELECT CASE WHEN r.device_status = 'rejected' THEN 'rejected_device' ELSE 'private_mac_device' END,
        CASE WHEN r.device_status = 'rejected' OR r.kind <> 'dhcp' THEN 'high' ELSE 'medium' END, r.segment_id, NULL::uuid, r.device_id, host(r.ip), r.mac::text,
        CASE WHEN r.device_status = 'rejected' THEN 'Rejected device ' ELSE 'Unapproved device with a private MAC ' END || r.mac || ' is active at ' || host(r.ip) || ' on ' || r.segment_name
 FROM recent r WHERE r.alert_private_macs AND r.is_private_mac AND NOT r.is_gateway AND r.device_status IS DISTINCT FROM 'approved'
 UNION ALL
 SELECT 'unregistered_static', 'medium', r.segment_id, NULL::uuid, NULL::uuid, host(r.ip), r.mac::text,
        'Static IP ' || host(r.ip) || ' is in use on ' || r.segment_name || ' but is not in the register'
 FROM recent r WHERE r.kind = 'dhcp' AND r.is_static AND r.register_id IS NULL AND NOT r.gateway_ip
 UNION ALL
 SELECT 'unknown_server_ip', CASE WHEN r.environment_severity = 'critical' THEN 'critical' ELSE 'high' END, r.segment_id, NULL::uuid, NULL::uuid,
        host(r.ip), r.mac::text, 'Unregistered IP ' || host(r.ip) || ' answers in server segment ' || r.segment_name
 FROM recent r WHERE r.kind IN ('server', 'cloud') AND r.register_id IS NULL AND NOT r.gateway_ip
 UNION ALL
 SELECT 'register_misuse', 'high', r.segment_id, r.register_id, r.device_id, host(r.ip), r.mac::text,
        CASE WHEN r.register_status = 'assigned' THEN host(r.ip) || ' is used by a different MAC than registered'
             ELSE host(r.ip) || ' is marked ' || r.register_status || ' but is in use' END
 FROM recent r WHERE r.register_id IS NOT NULL AND (r.register_status IN ('blocked', 'reserved')
   OR (r.kind = 'dhcp' AND r.register_status = 'assigned' AND r.expected_mac IS NOT NULL AND r.mac IS NOT NULL AND r.expected_mac <> r.mac))
 UNION ALL
 SELECT 'register_misuse', 'high', r.segment_id, NULL::uuid, NULL::uuid, host(r.ip), r.mac::text,
        'Reserved IP ' || host(r.ip) || ' is in use (' || hold.reason || ')'
 FROM recent r JOIN reservations hold ON hold.segment_id = r.segment_id AND hold.status = 'active' AND r.ip BETWEEN hold.start_ip AND hold.end_ip
 WHERE r.register_id IS NULL
 UNION ALL
 SELECT 'ip_conflict', 'high', (array_agg(segment_id))[1], NULL::uuid, NULL::uuid, host(ip), NULL::text,
        'IP conflict: ' || host(ip) || ' was seen with ' || count(DISTINCT mac) || ' MAC addresses'
 FROM recent WHERE mac IS NOT NULL AND (is_static OR last_seen > ip_latest - interval '5 minutes')
 GROUP BY ip HAVING count(DISTINCT mac) > 1
 UNION ALL
 SELECT 'server_mac_changed', CASE WHEN r.critical THEN 'critical' ELSE r.environment_severity END, r.segment_id, r.register_id, r.device_id,
        host(r.ip), r.mac::text, 'Server ' || COALESCE(NULLIF(r.assigned_name, ''), host(r.ip)) || ' answered with MAC ' || r.mac || ' instead of ' || r.expected_mac
 FROM recent r WHERE r.register_id IS NOT NULL AND r.kind IN ('server', 'cloud') AND r.expected_mac IS NOT NULL AND r.mac IS NOT NULL AND r.expected_mac <> r.mac
 UNION ALL
 SELECT 'server_down', CASE WHEN ip.critical THEN 'critical' ELSE COALESCE(env.default_severity, 'medium') END, sg.id, ip.id, NULL::uuid,
        host(ip.ip), NULL::text, 'Server ' || COALESCE(NULLIF(ip.assigned_name, ''), host(ip.ip)) || ' (' || host(ip.ip) || ') is down'
 FROM ip_addresses ip JOIN segments sg ON sg.id = ip.segment_id
 LEFT JOIN environments env ON env.id = COALESCE(ip.environment_id, sg.default_environment_id)
 WHERE ip.monitor AND ip.health = 'down'
 UNION ALL
 SELECT 'server_slow', 'medium', ip.segment_id, ip.id, NULL::uuid, host(ip.ip), NULL::text,
        'Server ' || COALESCE(NULLIF(ip.assigned_name, ''), host(ip.ip)) || ' (' || host(ip.ip) || ') is slow: it answered in ' || ip.response_ms || ' ms, over the ' || lim.ms || ' ms limit'
 FROM ip_addresses ip CROSS JOIN (SELECT COALESCE((SELECT (value #>> '{}')::integer FROM settings WHERE key = 'server_check.slow_ms'), 200) AS ms) lim
 WHERE ip.monitor AND ip.health = 'up' AND ip.response_ms > lim.ms
)
SELECT typ, severity, COALESCE(segment_id::text, ''), COALESCE(ipid::text, ''), COALESCE(device_id::text, ''),
       COALESCE(ip, ''), COALESCE(mac, ''), message FROM rules`

// pendingDevicesSQL records MACs nobody has decided on yet, so people can approve
// or reject them, whatever address DHCP gives them. Randomised (private) MACs are
// recorded only on segments that watch them: phones keep one per Wi-Fi network,
// but elsewhere they would only add noise.
const pendingDevicesSQL = `INSERT INTO devices (mac, status, first_seen, last_seen)
	SELECT v.mac, 'pending', min(v.first_seen), max(v.last_seen) FROM sightings v LEFT JOIN segments s ON s.id = v.segment_id
	WHERE v.mac IS NOT NULL AND (NOT v.is_private_mac OR (s.alert_private_macs AND NOT v.is_gateway))
	  AND v.last_seen > now() - interval '30 minutes' GROUP BY v.mac
	ON CONFLICT (mac) DO UPDATE SET first_seen = COALESCE(devices.first_seen, EXCLUDED.first_seen),
		last_seen = GREATEST(COALESCE(devices.last_seen, EXCLUDED.last_seen), EXCLUDED.last_seen)`

// upsertSQL opens alerts, or refreshes the open one for the same cause, in one
// statement. Per key it reports whether it is new, and whether the same cause closed
// within the last hour: a flapping cause reopens quietly instead of notifying again.
// A device alert follows its MAC, so its address and segment are refreshed.
const upsertSQL = `INSERT INTO alerts (alert_type, severity, segment_id, ip_address_id, device_id, ip, mac, message, dedupe_key)
	SELECT typ, sev, NULLIF(seg, '')::uuid, NULLIF(ipid, '')::uuid, NULLIF(dev, '')::uuid, NULLIF(ip, '')::inet, NULLIF(mac, '')::macaddr, msg, key
	FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[], $7::text[], $8::text[], $9::text[])
		AS v(typ, sev, seg, ipid, dev, ip, mac, msg, key)
	ON CONFLICT (dedupe_key) WHERE status <> 'closed' DO UPDATE SET last_seen_at = now(), message = EXCLUDED.message,
		severity = EXCLUDED.severity, ip = EXCLUDED.ip, segment_id = EXCLUDED.segment_id,
		ip_address_id = EXCLUDED.ip_address_id, device_id = EXCLUDED.device_id
	RETURNING dedupe_key, (xmax = 0), EXISTS (SELECT 1 FROM alerts a WHERE a.dedupe_key = alerts.dedupe_key
		AND a.status = 'closed' AND a.closed_at > now() - interval '1 hour')`

// closeSQL closes alerts whose cause is gone, except on a segment whose last scan
// failed or whose automatic scan is switched off: nobody looked, so the cause may
// still be there. Cloud and VPN segments are never swept, but the FortiGate's ARP
// table and VPN sessions still report on them, so their alerts close like any
// other when the evidence goes. It tells those who handle alerts when a
// server is back: only when it is still monitored and answering, not when it was
// released or its monitoring turned off.
const closeSQL = `WITH closed AS (
	UPDATE alerts SET status = 'closed', closed_at = now() WHERE status <> 'closed' AND NOT (dedupe_key = ANY($1::text[]))
		AND NOT (alert_type NOT IN ('server_down', 'server_slow') AND EXISTS (SELECT 1 FROM segments s WHERE s.id = alerts.segment_id
			AND (s.scan_failed_at IS NOT NULL OR (SELECT value = 'false'::jsonb FROM settings
				WHERE key = CASE s.kind WHEN 'dhcp' THEN 'scan.auto_devices' WHEN 'server' THEN 'scan.auto_servers' END))))
	RETURNING alert_type, ip
) INSERT INTO notifications (user_id, kind, message, link)
SELECT u.id, 'server_up', 'Server ' || COALESCE(NULLIF(i.assigned_name, ''), host(closed.ip)) || ' (' || host(closed.ip) || ') is back up', '/servers'
FROM closed JOIN ip_addresses i ON i.ip = closed.ip AND i.monitor AND i.health = 'up' CROSS JOIN users u
WHERE closed.alert_type = 'server_down' AND u.is_active
	AND EXISTS (SELECT 1 FROM roles r WHERE r.key = u.role AND (r.full_access OR 'alerts:manage' = ANY(r.permissions)))`
