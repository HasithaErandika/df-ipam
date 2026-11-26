-- Email is kept for what someone must act on. Each notice says how it is
-- emailed: now (its own message), digest (gathered every few minutes) or not at
-- all; a daily summary is built from what is open. A notice with no user is an
-- email for the team mailbox (Settings: email.team_address): no inbox shows it.
ALTER TABLE notifications
  ALTER COLUMN user_id DROP NOT NULL,
  ADD COLUMN email_mode text NOT NULL DEFAULT '' CHECK (email_mode IN ('', 'now', 'digest')),
  ADD COLUMN severity text NOT NULL DEFAULT '',
  ADD CONSTRAINT notifications_team_is_email CHECK (user_id IS NOT NULL OR (email_mode <> '' AND is_read));

-- Email queued the old way would arrive one by one: leave it in the app.
UPDATE notifications SET email_status = 'none' WHERE email_status = 'pending';

DROP INDEX notifications_outbox_idx;
CREATE INDEX notifications_outbox_idx ON notifications (email_mode, created_at) WHERE email_status = 'pending';
