-- More about each device, all evidence:
-- the NetBIOS workgroup or Windows domain; what Windows says when a file
-- sharing (SMB) connection starts (its domain, full DNS name and version);
-- and from Microsoft Intune, the device's serial, maker, model, OS,
-- compliance, disk encryption and last check-in.
ALTER TABLE sightings ADD COLUMN workgroup text NOT NULL DEFAULT '',
  ADD COLUMN windows_domain text NOT NULL DEFAULT '',
  ADD COLUMN windows_fqdn text NOT NULL DEFAULT '',
  ADD COLUMN os_version text NOT NULL DEFAULT '',
  ADD COLUMN intune_serial text NOT NULL DEFAULT '',
  ADD COLUMN intune_maker text NOT NULL DEFAULT '',
  ADD COLUMN intune_model text NOT NULL DEFAULT '',
  ADD COLUMN intune_os text NOT NULL DEFAULT '',
  ADD COLUMN intune_compliance text NOT NULL DEFAULT '',
  ADD COLUMN intune_encrypted boolean,
  ADD COLUMN intune_last_sync timestamptz;
