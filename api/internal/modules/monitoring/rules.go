package monitoring

// candidatesSQL lists every alert cause that is true right now (docs/new.md, section 5).
// "Recent" follows the scan interval, so alerts don't close between two slow scans.
// Private (randomised) MACs never count as unknown devices.
const candidatesSQL = `
WITH win AS (
 SELECT make_interval(mins => GREATEST(10, 2 * COALESCE((SELECT (value #>> '{}')::integer FROM settings WHERE key = 'scan.interval_minutes'), 5))) AS w
), recent AS (
 SELECT si.*, sg.kind, sg.name AS segment_name, sg.alert_private_macs, COALESCE(env.name, '') AS environment_name,
        COALESCE(env.default_severity, 'medium') AS environment_severity, dev.id AS device_id, dev.status AS device_status,
        ipr.id AS register_id, ipr.status AS register_status, ipr.expected_mac, ipr.critical, ipr.assigned_name
 FROM sightings si JOIN segments sg ON sg.id = si.segment_id
 LEFT JOIN devices dev ON dev.mac = si.mac
 LEFT JOIN ip_addresses ipr ON ipr.ip = si.ip
 LEFT JOIN environments env ON env.id = COALESCE(ipr.environment_id, sg.default_environment_id)
 WHERE si.last_seen > now() - (SELECT w FROM win)
), rules AS (
 SELECT CASE WHEN r.device_status = 'rejected' THEN 'rejected_device' ELSE 'unknown_device' END AS typ,
        CASE WHEN r.device_status = 'rejected' OR r.kind IN ('server', 'cloud') THEN 'high' ELSE 'medium' END AS severity,
        r.segment_id, r.register_id AS ipid, r.device_id, host(r.ip) AS ip, r.mac::text AS mac,
        CASE WHEN r.device_status = 'rejected' THEN 'Rejected device ' ELSE 'Unapproved device ' END || r.mac || ' is active at ' || host(r.ip) || ' on ' || r.segment_name AS message
 FROM recent r WHERE r.mac IS NOT NULL AND NOT r.is_private_mac AND r.device_status IS DISTINCT FROM 'approved'
 UNION ALL
 SELECT 'private_mac_device', CASE WHEN r.kind = 'dhcp' THEN 'medium' ELSE 'high' END, r.segment_id, NULL::uuid, NULL::uuid, host(r.ip), NULL::text,
        'Device with a private MAC (' || r.mac || ') is active at ' || host(r.ip) || ' on ' || r.segment_name
 FROM recent r WHERE r.alert_private_macs AND r.is_private_mac AND NOT r.is_gateway AND r.register_id IS NULL
 UNION ALL
 SELECT 'unregistered_static', 'medium', r.segment_id, NULL::uuid, NULL::uuid, host(r.ip), r.mac::text,
        'Static IP ' || host(r.ip) || ' is in use on ' || r.segment_name || ' but is not in the register'
 FROM recent r WHERE r.kind = 'dhcp' AND r.is_static AND r.register_id IS NULL
 UNION ALL
 SELECT 'unknown_server_ip', CASE WHEN r.environment_severity = 'critical' THEN 'critical' ELSE 'high' END, r.segment_id, NULL::uuid, NULL::uuid,
        host(r.ip), r.mac::text, 'Unregistered IP ' || host(r.ip) || ' answers in server segment ' || r.segment_name
 FROM recent r WHERE r.kind IN ('server', 'cloud') AND r.register_id IS NULL
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
 FROM recent WHERE mac IS NOT NULL GROUP BY ip HAVING count(DISTINCT mac) > 1
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
)
SELECT typ, severity, COALESCE(segment_id::text, ''), COALESCE(ipid::text, ''), COALESCE(device_id::text, ''),
       COALESCE(ip, ''), COALESCE(mac, ''), message FROM rules`

// pendingDevicesSQL records MACs nobody has decided on yet, so people can approve
// or reject them. Randomised MACs are skipped: they change and would flood the list.
const pendingDevicesSQL = `INSERT INTO devices (mac, status, first_seen, last_seen)
	SELECT mac, 'pending', min(first_seen), max(last_seen) FROM sightings
	WHERE mac IS NOT NULL AND NOT is_private_mac AND last_seen > now() - interval '30 minutes' GROUP BY mac
	ON CONFLICT (mac) DO UPDATE SET first_seen = COALESCE(devices.first_seen, EXCLUDED.first_seen),
		last_seen = GREATEST(COALESCE(devices.last_seen, EXCLUDED.last_seen), EXCLUDED.last_seen)`

// upsertSQL opens an alert, or refreshes the open one for the same cause. It reports
// whether it is new, and whether the same cause closed within the last hour: a
// flapping cause reopens quietly instead of notifying again.
const upsertSQL = `INSERT INTO alerts (alert_type, severity, segment_id, ip_address_id, device_id, ip, mac, message, dedupe_key)
	VALUES ($1, $2, NULLIF($3, '')::uuid, NULLIF($4, '')::uuid, NULLIF($5, '')::uuid, NULLIF($6, '')::inet, NULLIF($7, '')::macaddr, $8, $9)
	ON CONFLICT (dedupe_key) WHERE status <> 'closed' DO UPDATE SET last_seen_at = now(), message = EXCLUDED.message, severity = EXCLUDED.severity
	RETURNING (xmax = 0), EXISTS (SELECT 1 FROM alerts a WHERE a.dedupe_key = $9 AND a.status = 'closed' AND a.closed_at > now() - interval '1 hour')`

// closeSQL closes alerts whose cause is gone, and tells Admins when a server is back.
const closeSQL = `WITH closed AS (
	UPDATE alerts SET status = 'closed', closed_at = now() WHERE status <> 'closed' AND NOT (dedupe_key = ANY($1::text[]))
	RETURNING alert_type, ip
) INSERT INTO notifications (user_id, kind, message, link, email_status)
SELECT u.id, 'server_up', 'Server ' || COALESCE(NULLIF(i.assigned_name, ''), host(closed.ip)) || ' (' || host(closed.ip) || ') is back up', '/servers',
	CASE WHEN COALESCE(p.email, true) THEN 'pending' ELSE 'none' END
FROM closed LEFT JOIN ip_addresses i ON i.ip = closed.ip CROSS JOIN users u
LEFT JOIN notification_preferences p ON p.user_id = u.id AND p.kind = 'server_up'
WHERE closed.alert_type = 'server_down' AND u.role = 'admin' AND u.is_active`
