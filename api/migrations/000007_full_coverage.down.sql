DROP TABLE retention_policies, notification_preferences, reconciliation_rules, discovery_blackouts,
  agent_registration_tokens, firewall_address_group_members, firewall_address_groups, firewall_address_objects,
  network_routes, cloud_networks, cloud_accounts, routing_domain_peerings, subnet_reserved_ranges, sessions;
DROP INDEX ip_reservations_expiry_idx, ip_requests_requester_idx, ip_requests_status_idx, devices_site_idx,
  device_interfaces_mac_idx, ip_addresses_owner_idx, ip_addresses_device_idx, subnets_business_service_idx,
  subnets_environment_idx, subnets_vdom_idx, subnets_vlan_idx, subnets_site_idx;
ALTER TABLE subnets DROP COLUMN business_unit_id;
