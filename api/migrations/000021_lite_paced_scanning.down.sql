ALTER TABLE settings DROP CONSTRAINT settings_intune_flag;
ALTER TABLE settings DROP CONSTRAINT settings_known_numbers;
ALTER TABLE settings ADD CONSTRAINT settings_known_numbers CHECK (
  key NOT IN ('scan.interval_minutes', 'server_check.interval_seconds', 'server_check.down_after_failures',
              'reservations.reminder_days', 'session.idle_timeout_minutes', 'fortigate.interval_minutes')
  OR jsonb_typeof(value) = 'number');
CREATE OR REPLACE FUNCTION seen_window() RETURNS interval LANGUAGE sql STABLE AS $$
  SELECT make_interval(mins => GREATEST(10, 2 * COALESCE(
    (SELECT (value #>> '{}')::integer FROM settings WHERE key = 'scan.interval_minutes'), 10)))
$$;
DELETE FROM settings WHERE key IN ('scan.server_interval_minutes', 'scan.rate_per_second', 'scan.port_check_hours');
UPDATE settings SET value = '3600' WHERE key = 'server_check.interval_seconds' AND value = '300';
UPDATE settings SET value = '10' WHERE key = 'scan.interval_minutes' AND value = '15';
ALTER TABLE sightings DROP COLUMN ports_checked_at;
