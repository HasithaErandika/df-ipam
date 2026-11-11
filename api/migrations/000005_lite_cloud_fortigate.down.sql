ALTER TABLE settings DROP CONSTRAINT IF EXISTS settings_fortigate_flags;
ALTER TABLE settings DROP CONSTRAINT IF EXISTS settings_known_numbers;
ALTER TABLE settings ADD CONSTRAINT settings_known_numbers CHECK (
  key NOT IN ('scan.interval_minutes', 'server_check.interval_seconds', 'server_check.down_after_failures',
              'reservations.reminder_days', 'session.idle_timeout_minutes')
  OR jsonb_typeof(value) = 'number');
DELETE FROM settings WHERE key LIKE 'fortigate.%';
DELETE FROM sightings WHERE source IN ('dhcp', 'vpn');
ALTER TABLE sightings DROP CONSTRAINT sightings_source_check;
ALTER TABLE sightings ADD CONSTRAINT sightings_source_check CHECK (source IN ('arp', 'ping', 'netbios', 'snmp', 'dns'));
ALTER TABLE segments DROP CONSTRAINT segments_environment_kind;
ALTER TABLE segments ADD CONSTRAINT segments_check6 CHECK (kind = 'server' OR default_environment_id IS NULL);
ALTER TABLE segments DROP CONSTRAINT segments_kind_check;
ALTER TABLE segments ADD CONSTRAINT segments_kind_check CHECK (kind IN ('dhcp', 'server'));
