-- Monitored servers are checked every hour by default, not every 5 minutes.
-- A value someone chose in Settings is left alone.
UPDATE settings SET value = '3600' WHERE key = 'server_check.interval_seconds' AND value = '300';
