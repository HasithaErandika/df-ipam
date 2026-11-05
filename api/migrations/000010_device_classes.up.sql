-- More device types, so endpoints and wireless gear can be told apart, and an
-- operating system family recorded separately from the free-text version.
-- os_family is set from a record or a reliable source, never guessed.
ALTER TABLE devices DROP CONSTRAINT devices_device_type_check;
ALTER TABLE devices ADD CONSTRAINT devices_device_type_check CHECK (device_type IN
  ('firewall','router','switch','physical_server','virtual_machine','load_balancer','network_appliance','cloud_interface',
   'access_point','wireless_controller','laptop','desktop','mobile_phone','tablet','printer','ip_phone','camera','storage','iot','other'));
ALTER TABLE devices ADD COLUMN os_family text NOT NULL DEFAULT 'unknown' CHECK (os_family IN
  ('unknown','windows','windows_server','linux','macos','ios','android','network_os','hypervisor','other'));
CREATE INDEX devices_type_idx ON devices (device_type);
