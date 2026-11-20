-- The device's user from Microsoft Entra ID, for devices joined to or
-- registered in Entra but not enrolled in Intune: the registered owner's
-- sign-in email and name, the Entra device name and join type, and when the
-- device last signed in. Matched by computer name only (Entra keeps no MAC).
-- Evidence only: it never sets a register entry's or device's owner.
ALTER TABLE sightings ADD COLUMN entra_user text NOT NULL DEFAULT '',
  ADD COLUMN entra_user_name text NOT NULL DEFAULT '',
  ADD COLUMN entra_device text NOT NULL DEFAULT '',
  ADD COLUMN entra_join text NOT NULL DEFAULT '',
  ADD COLUMN entra_last_sign_in timestamptz,
  ADD COLUMN entra_seen_at timestamptz;
