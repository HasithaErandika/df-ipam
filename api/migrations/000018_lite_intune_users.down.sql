DELETE FROM settings WHERE key LIKE 'intune.%';
ALTER TABLE sightings DROP COLUMN intune_seen_at, DROP COLUMN intune_device,
  DROP COLUMN intune_user_name, DROP COLUMN intune_user;
