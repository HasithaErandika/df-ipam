-- Indexes for the Discovery findings, alert and audit filters. No new tables.
CREATE INDEX alerts_ip_open_idx ON alerts (ip) WHERE status <> 'closed';
CREATE INDEX alerts_closed_key_idx ON alerts (dedupe_key, closed_at) WHERE status = 'closed';
CREATE INDEX sightings_last_seen_idx ON sightings (last_seen DESC);
CREATE INDEX audit_logs_actor_idx ON audit_logs (actor_id, occurred_at DESC);
CREATE INDEX notifications_sent_idx ON notifications (user_id, created_at) WHERE email_status = 'sent';

-- Alert email goes to Admins by their own preferences; this list was never used.
DELETE FROM settings WHERE key = 'alerts.email_recipients';

-- Settings that Admins change, with their limits enforced in the database too.
ALTER TABLE settings ADD CONSTRAINT settings_known_numbers CHECK (
  key NOT IN ('scan.interval_minutes', 'server_check.interval_seconds', 'server_check.down_after_failures',
              'reservations.reminder_days', 'session.idle_timeout_minutes')
  OR jsonb_typeof(value) = 'number');
ALTER TABLE settings ADD CONSTRAINT settings_email_flag CHECK (key <> 'email.enabled' OR jsonb_typeof(value) = 'boolean');
