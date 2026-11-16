UPDATE settings SET value = '86400' WHERE key = 'server_check.interval_seconds' AND value = '300';
DROP FUNCTION seen_window();
ALTER TABLE sightings DROP COLUMN identified_at;
