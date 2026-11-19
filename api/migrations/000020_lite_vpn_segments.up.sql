-- VPN client pools as their own kind of segment: addresses the FortiGate hands
-- to VPN users. Like cloud networks they are never swept from the building:
-- the FortiGate's VPN sessions are what shows them.
ALTER TABLE segments DROP CONSTRAINT segments_kind_check;
ALTER TABLE segments ADD CONSTRAINT segments_kind_check CHECK (kind IN ('dhcp', 'server', 'cloud', 'vpn'));

-- The VPN user and where they connected from, kept apart from the hostname.
ALTER TABLE sightings ADD COLUMN vpn_user text NOT NULL DEFAULT '',
  ADD COLUMN vpn_kind text NOT NULL DEFAULT '' CHECK (vpn_kind IN ('', 'ssl', 'ipsec')),
  ADD COLUMN vpn_tunnel text NOT NULL DEFAULT '',
  ADD COLUMN vpn_remote text NOT NULL DEFAULT '';

-- VPN sightings so far carried the user inside the hostname ("SSL VPN: nimal
-- from …"). They are evidence of sessions, read again on the next FortiGate read.
DELETE FROM sightings WHERE source = 'vpn';
