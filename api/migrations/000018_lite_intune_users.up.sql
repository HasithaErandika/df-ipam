-- The device's user from Microsoft Intune: the primary user's sign-in email and
-- name, matched to a sighting by MAC (or by computer name). Evidence only: it
-- never sets a register entry's or device's owner.
ALTER TABLE sightings ADD COLUMN intune_user text NOT NULL DEFAULT '',
  ADD COLUMN intune_user_name text NOT NULL DEFAULT '',
  ADD COLUMN intune_device text NOT NULL DEFAULT '',
  ADD COLUMN intune_seen_at timestamptz;
