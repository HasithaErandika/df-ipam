DROP INDEX notifications_outbox_idx;
CREATE INDEX notifications_outbox_idx ON notifications (created_at) WHERE email_status = 'pending';
DELETE FROM notifications WHERE user_id IS NULL;
ALTER TABLE notifications
  DROP CONSTRAINT notifications_team_is_email,
  DROP COLUMN severity,
  DROP COLUMN email_mode,
  ALTER COLUMN user_id SET NOT NULL;
