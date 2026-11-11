DROP TABLE IF EXISTS audit_logs, settings, notification_preferences, notifications, alerts, sightings,
  ip_requests, ip_addresses, reservations, devices, segments, environments, users CASCADE;
DROP FUNCTION IF EXISTS audit_logs_immutable(), check_segment_contents(), check_range_in_segment(), check_ip_in_segment(), set_updated_at();
