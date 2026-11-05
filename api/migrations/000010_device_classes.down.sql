DROP INDEX devices_type_idx;
ALTER TABLE devices DROP COLUMN os_family;
UPDATE devices SET device_type = 'other' WHERE device_type IN
  ('access_point','wireless_controller','laptop','desktop','mobile_phone','tablet','printer','ip_phone','camera','storage','iot');
ALTER TABLE devices DROP CONSTRAINT devices_device_type_check;
ALTER TABLE devices ADD CONSTRAINT devices_device_type_check CHECK (device_type IN
  ('firewall','router','switch','physical_server','virtual_machine','load_balancer','network_appliance','cloud_interface','other'));
