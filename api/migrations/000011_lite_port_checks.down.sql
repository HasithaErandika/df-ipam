DROP INDEX ip_addresses_device_idx;
DROP INDEX ip_addresses_expected_mac_idx;
ALTER TABLE sightings DROP COLUMN open_ports;
ALTER TABLE segments DROP COLUMN check_ports;
