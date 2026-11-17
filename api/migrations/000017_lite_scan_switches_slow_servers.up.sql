-- How long a monitored server took to answer its last check, in milliseconds
-- (the faster of two pings, or the check port's connect time). NULL when down.
ALTER TABLE ip_addresses ADD COLUMN response_ms integer CHECK (response_ms >= 0);

-- A server that answers, but slower than server_check.slow_ms.
ALTER TABLE alerts DROP CONSTRAINT alerts_alert_type_check;
ALTER TABLE alerts ADD CONSTRAINT alerts_alert_type_check CHECK (alert_type IN ('unknown_device', 'unregistered_static',
  'register_misuse', 'unknown_server_ip', 'ip_conflict', 'server_down', 'server_mac_changed', 'rejected_device', 'private_mac_device',
  'server_slow'));
