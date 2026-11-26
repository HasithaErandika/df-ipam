-- A failed email waits longer before each retry (1, 2, 4, 8 minutes), so a
-- short SMTP outage no longer uses up every attempt within half a minute.
ALTER TABLE notifications ADD COLUMN email_next_at timestamptz;
