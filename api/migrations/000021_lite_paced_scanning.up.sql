-- Scanning sized for about 1,000 users and 250 servers, without bursts.

-- When a device's ports were last checked: every 24 hours by default, apart
-- from the 6-hourly identification.
ALTER TABLE sightings ADD COLUMN ports_checked_at timestamptz;
UPDATE sightings SET ports_checked_at = identified_at WHERE identified_at IS NOT NULL AND cardinality(open_ports) > 0;

-- User networks every 15 minutes (was 10), servers on their own 10-minute
-- schedule, monitored servers checked every 5 minutes (was hourly), at most
-- 30 probes a second shared by all of it. Only values still at the old
-- defaults change; anything an Admin chose stays.
UPDATE settings SET value = '15' WHERE key = 'scan.interval_minutes' AND value = '10';
UPDATE settings SET value = '300' WHERE key = 'server_check.interval_seconds' AND value = '3600';
INSERT INTO settings (key, value) VALUES ('scan.server_interval_minutes', '10'), ('scan.rate_per_second', '30'), ('scan.port_check_hours', '24')
ON CONFLICT (key) DO NOTHING;

-- "Seen recently" follows the slower of the two schedules.
CREATE OR REPLACE FUNCTION seen_window() RETURNS interval LANGUAGE sql STABLE AS $$
  SELECT make_interval(mins => GREATEST(10, 2 * GREATEST(
    COALESCE((SELECT (value #>> '{}')::integer FROM settings WHERE key = 'scan.interval_minutes'), 15),
    COALESCE((SELECT (value #>> '{}')::integer FROM settings WHERE key = 'scan.server_interval_minutes'), 10))))
$$;

-- The new numbers, and the Intune switch and interval, must be the right type.
ALTER TABLE settings DROP CONSTRAINT settings_known_numbers;
ALTER TABLE settings ADD CONSTRAINT settings_known_numbers CHECK (
  key NOT IN ('scan.interval_minutes', 'server_check.interval_seconds', 'server_check.down_after_failures',
              'reservations.reminder_days', 'session.idle_timeout_minutes', 'fortigate.interval_minutes',
              'scan.server_interval_minutes', 'scan.rate_per_second', 'scan.port_check_hours', 'intune.interval_minutes')
  OR jsonb_typeof(value) = 'number');
ALTER TABLE settings ADD CONSTRAINT settings_intune_flag CHECK (key <> 'intune.enabled' OR jsonb_typeof(value) = 'boolean');
