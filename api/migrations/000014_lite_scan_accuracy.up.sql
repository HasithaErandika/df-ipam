-- Scan accuracy and steadier scanning.

-- When each host was last identified in full (mDNS, UPnP, ports), so a restart
-- doesn't identify every host on the network again at once.
ALTER TABLE sightings ADD COLUMN identified_at timestamptz;

-- "Seen recently": answered within two scan intervals, at least 10 minutes.
-- One definition for the alert rules, Discovery, segments and the dashboard.
CREATE FUNCTION seen_window() RETURNS interval LANGUAGE sql STABLE AS $$
  SELECT make_interval(mins => GREATEST(10, 2 * COALESCE(
    (SELECT (value #>> '{}')::integer FROM settings WHERE key = 'scan.interval_minutes'), 10)))
$$;
REVOKE ALL ON FUNCTION seen_window() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION seen_window() TO dfipam_app;

-- Monitored servers are checked every 5 minutes, not once a day: with three
-- failures before Down, a daily check took three days to notice an outage.
UPDATE settings SET value = '300' WHERE key = 'server_check.interval_seconds' AND value = '86400';
