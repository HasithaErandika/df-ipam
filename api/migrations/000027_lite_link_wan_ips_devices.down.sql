ALTER TABLE network_links ADD COLUMN device_id uuid REFERENCES devices(id);
UPDATE network_links SET device_id = device_ids[1] WHERE cardinality(device_ids) > 0;
DROP INDEX network_links_devices_idx;
ALTER TABLE network_links DROP COLUMN device_ids;
CREATE INDEX network_links_device_idx ON network_links (device_id);
ALTER TABLE network_links DROP COLUMN extra_wan_ips;
DROP FUNCTION all_ipv4_inet(inet[]);
