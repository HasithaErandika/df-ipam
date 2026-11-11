-- Cloud segments: networks hosted in a cloud. Static like server segments, with a
-- default environment, but never ping-swept from the building.
ALTER TABLE segments DROP CONSTRAINT segments_kind_check;
ALTER TABLE segments ADD CONSTRAINT segments_kind_check CHECK (kind IN ('dhcp', 'server', 'cloud'));
ALTER TABLE segments DROP CONSTRAINT segments_check6;
ALTER TABLE segments ADD CONSTRAINT segments_environment_kind CHECK (kind IN ('server', 'cloud') OR default_environment_id IS NULL);

-- Evidence read from the FortiGate (read-only): DHCP leases and VPN sessions.
-- ARP entries from it are recorded as 'arp'.
ALTER TABLE sightings DROP CONSTRAINT sightings_source_check;
ALTER TABLE sightings ADD CONSTRAINT sightings_source_check CHECK (source IN ('arp', 'ping', 'netbios', 'snmp', 'dns', 'dhcp', 'vpn'));

-- New defaults: devices every 10 minutes, servers once a day. Only values still at
-- the old defaults move; anything an Admin chose stays.
UPDATE settings SET value = '10' WHERE key = 'scan.interval_minutes' AND value = '5';
UPDATE settings SET value = '86400' WHERE key = 'server_check.interval_seconds' AND value = '60';

ALTER TABLE settings DROP CONSTRAINT settings_known_numbers;
ALTER TABLE settings ADD CONSTRAINT settings_known_numbers CHECK (
  key NOT IN ('scan.interval_minutes', 'server_check.interval_seconds', 'server_check.down_after_failures',
              'reservations.reminder_days', 'session.idle_timeout_minutes', 'fortigate.interval_minutes')
  OR jsonb_typeof(value) = 'number');
ALTER TABLE settings ADD CONSTRAINT settings_fortigate_flags CHECK (
  key NOT IN ('fortigate.enabled', 'fortigate.verify_tls') OR jsonb_typeof(value) = 'boolean');
