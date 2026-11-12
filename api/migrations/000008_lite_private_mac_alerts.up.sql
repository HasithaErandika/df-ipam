-- Per segment: raise an alert when a device with a private (randomised) MAC is
-- active at an address that isn't registered. Off by default, because phones
-- change these MACs; one alert per address keeps it from flooding.
ALTER TABLE segments ADD COLUMN alert_private_macs boolean NOT NULL DEFAULT false;

ALTER TABLE alerts DROP CONSTRAINT alerts_alert_type_check;
ALTER TABLE alerts ADD CONSTRAINT alerts_alert_type_check CHECK (alert_type IN ('unknown_device', 'unregistered_static',
  'register_misuse', 'unknown_server_ip', 'ip_conflict', 'server_down', 'server_mac_changed', 'rejected_device', 'private_mac_device'));
