-- A link can have more than one WAN address: an ILL with two public /32s, say.
-- wan_ip and wan_segment_id stay the main address (the firewall matches on it);
-- the others are kept here, each with its subnet's prefix (122.255.35.1/32,
-- 10.10.40.253/24), so the subnet is known without a segment in the register.
CREATE FUNCTION all_ipv4_inet(addrs inet[]) RETURNS boolean LANGUAGE sql IMMUTABLE AS $$
  SELECT COALESCE(bool_and(family(a) = 4), true) FROM unnest(addrs) a
$$;

ALTER TABLE network_links
  ADD COLUMN extra_wan_ips inet[] NOT NULL DEFAULT '{}'
    CHECK (cardinality(extra_wan_ips) <= 16 AND all_ipv4_inet(extra_wan_ips));

-- A link can have several connected devices (the firewall, the provider's
-- router, a switch). device_ids replaces the single device_id. Devices are
-- never deleted, so the ids stay valid; the API checks new ones are approved.
ALTER TABLE network_links ADD COLUMN device_ids uuid[] NOT NULL DEFAULT '{}' CHECK (cardinality(device_ids) <= 50);
UPDATE network_links SET device_ids = ARRAY[device_id] WHERE device_id IS NOT NULL;
DROP INDEX network_links_device_idx;
ALTER TABLE network_links DROP COLUMN device_id;
CREATE INDEX network_links_devices_idx ON network_links USING gin (device_ids);
