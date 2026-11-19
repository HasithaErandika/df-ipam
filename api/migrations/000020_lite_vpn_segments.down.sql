ALTER TABLE sightings DROP COLUMN vpn_remote, DROP COLUMN vpn_tunnel, DROP COLUMN vpn_kind, DROP COLUMN vpn_user;
UPDATE segments SET kind = 'cloud' WHERE kind = 'vpn';
ALTER TABLE segments DROP CONSTRAINT segments_kind_check;
ALTER TABLE segments ADD CONSTRAINT segments_kind_check CHECK (kind IN ('dhcp', 'server', 'cloud'));
