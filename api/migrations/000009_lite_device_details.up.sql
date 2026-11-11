-- What a device says about itself, kept apart from its network card. nic_vendor
-- is the maker named by the MAC's first half (the card, not the device). maker,
-- model and device_kind come from UPnP or mDNS, and os_hint from the ping reply's
-- TTL. All are evidence only: never written to the register or devices.
ALTER TABLE sightings
  ADD COLUMN nic_vendor  text NOT NULL DEFAULT '',
  ADD COLUMN maker       text NOT NULL DEFAULT '',
  ADD COLUMN model       text NOT NULL DEFAULT '',
  ADD COLUMN device_kind text NOT NULL DEFAULT '',
  ADD COLUMN os_hint     text NOT NULL DEFAULT '';
