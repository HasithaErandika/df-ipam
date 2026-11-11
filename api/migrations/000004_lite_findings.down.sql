ALTER TABLE settings DROP CONSTRAINT IF EXISTS settings_email_flag;
ALTER TABLE settings DROP CONSTRAINT IF EXISTS settings_known_numbers;
INSERT INTO settings (key, value) VALUES ('alerts.email_recipients', '[]') ON CONFLICT (key) DO NOTHING;
DROP INDEX IF EXISTS notifications_sent_idx;
DROP INDEX IF EXISTS audit_logs_actor_idx;
DROP INDEX IF EXISTS sightings_last_seen_idx;
DROP INDEX IF EXISTS alerts_closed_key_idx;
DROP INDEX IF EXISTS alerts_ip_open_idx;
