DELETE FROM alerts WHERE alert_type = 'server_slow';
ALTER TABLE alerts DROP CONSTRAINT alerts_alert_type_check;
ALTER TABLE alerts ADD CONSTRAINT alerts_alert_type_check CHECK (alert_type IN ('unknown_device', 'unregistered_static',
  'register_misuse', 'unknown_server_ip', 'ip_conflict', 'server_down', 'server_mac_changed', 'rejected_device', 'private_mac_device'));
ALTER TABLE ip_addresses DROP COLUMN response_ms;
