-- Per segment: during each scan, try a short fixed list of common TCP ports on
-- every device that answers, and keep which are open. A connect that is closed
-- at once, with nothing sent; still, security tools may see it as a port scan,
-- so it is off by default. Evidence only, replaced on every scan.
ALTER TABLE segments ADD COLUMN check_ports boolean NOT NULL DEFAULT false;
ALTER TABLE sightings ADD COLUMN open_ports integer[] NOT NULL DEFAULT '{}' CHECK (cardinality(open_ports) <= 64);

-- Discovery looks up where a device is registered when it turns up at another
-- address (a laptop moving between Wi-Fi networks), by MAC or by device.
CREATE INDEX ip_addresses_expected_mac_idx ON ip_addresses (expected_mac) WHERE expected_mac IS NOT NULL;
CREATE INDEX ip_addresses_device_idx ON ip_addresses (device_id) WHERE device_id IS NOT NULL;
