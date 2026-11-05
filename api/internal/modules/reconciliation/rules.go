package reconciliation

// Each rule selects findings. Rules only read inventory and evidence: the only table
// a run writes is reconciliation_results. Results are always matched within a
// routing domain, so the same private IP in two domains is never a conflict.
type rule struct{ name, sql string }

// classOf gives the classification a rule produces, so an administrator can switch
// off a classification without touching the rules behind it.
var classOf = map[string]string{
	"subnet_overlap": "subnet_overlap", "subnet_ownership": "ownership_missing", "address_ownership": "ownership_missing",
	"observed_not_allocated": "observed_not_allocated", "allocated_not_observed": "allocated_not_observed", "stale_evidence": "stale_evidence",
	"unknown_ip": "unknown_ip", "conflict_suspected": "ip_conflict_suspected", "duplicate_mac": "duplicate_mac", "dns_mismatch": "dns_mismatch",
	"vlan_site_mismatch": "infrastructure_mismatch", "interface_device_mismatch": "infrastructure_mismatch",
	"cloud_space_missing": "infrastructure_mismatch", "new_device": "new_device",
}

var rules = []rule{
	{"subnet_overlap", `
	SELECT md5('overlap' || a.id || b.id) fingerprint, 'subnet_overlap' classification, 'subnet' object_type, a.id object_id,
	  a.routing_domain_id, NULL::inet address, ARRAY['Inventory'] sources, a.created_at evidence_from, now() evidence_to, 100 source_reliability, 95 confidence,
	  a.cidr || ' (' || a.name || ') overlaps ' || b.cidr || ' (' || b.name || ') with no valid exception.' explanation,
	  'Renew the overlap exception, or renumber one of the subnets.' suggested_action
	FROM subnets a JOIN subnets b ON a.routing_domain_id = b.routing_domain_id AND a.id < b.id AND a.cidr && b.cidr
	WHERE a.archived_at IS NULL AND b.archived_at IS NULL AND NOT EXISTS (
	  SELECT 1 FROM overlap_exceptions e WHERE e.id IN (a.overlap_exception_id, b.overlap_exception_id) AND (e.expires_at IS NULL OR e.expires_at > now()))`},

	{"subnet_ownership", `
	SELECT md5('owner-subnet' || s.id), 'ownership_missing', 'subnet', s.id, s.routing_domain_id, NULL::inet, ARRAY['Inventory'], s.created_at, now(), 100, 100,
	  s.cidr || ' (' || s.name || ') has no ' ||
	  CASE WHEN s.technical_owner_id IS NULL AND s.business_owner_id IS NULL THEN 'technical or business owner'
	       WHEN s.technical_owner_id IS NULL THEN 'technical owner' ELSE 'business owner' END || '.',
	  'Assign the missing owner on the subnet.'
	FROM subnets s WHERE s.archived_at IS NULL AND (s.technical_owner_id IS NULL OR s.business_owner_id IS NULL)`},

	{"address_ownership", `
	SELECT md5('owner-ip' || i.id), 'ownership_missing', 'ip_address', i.id, i.routing_domain_id, i.address, ARRAY['Inventory'], i.created_at, now(), 100, 100,
	  'Allocated address ' || host(i.address) || ' has no owner.', 'Assign an owner to this address.'
	FROM ip_addresses i WHERE i.allocation_state = 'allocated' AND i.owner_id IS NULL`},

	{"observed_not_allocated", `
	SELECT md5('observed' || i.id), 'observed_not_allocated', 'ip_address', i.id, i.routing_domain_id, i.address, ARRAY[COALESCE(os.name, 'Discovery')],
	  i.first_seen, i.last_seen, COALESCE(os.reliability, 50), COALESCE(os.reliability, 50),
	  host(i.address) || ' is active but has no allocation.', 'Find who is using this address, then allocate or investigate it.'
	FROM ip_addresses i LEFT JOIN observation_sources os ON os.id = i.discovery_source_id
	WHERE i.allocation_state = 'available_candidate' AND i.observation_state = 'observed_active'`},

	{"allocated_not_observed", `
	SELECT md5('silent' || i.id), 'allocated_not_observed', 'ip_address', i.id, i.routing_domain_id, i.address, ARRAY[COALESCE(os.name, 'Discovery')],
	  i.first_seen, i.last_seen, COALESCE(os.reliability, 50), 40,
	  host(i.address) || ' is allocated but was last seen ' || to_char(i.last_seen, 'YYYY-MM-DD') || '. This does not mean it is free.',
	  'Confirm with the owner. The allocation stays in place until a person changes it.'
	FROM ip_addresses i LEFT JOIN observation_sources os ON os.id = i.discovery_source_id
	WHERE i.allocation_state = 'allocated' AND i.last_seen < now() - interval '30 days'`},

	{"stale_evidence", `
	SELECT md5('stale' || i.id), 'stale_evidence', 'ip_address', i.id, i.routing_domain_id, i.address, ARRAY[COALESCE(os.name, 'Discovery')],
	  i.first_seen, i.last_seen, COALESCE(os.reliability, 50), 60,
	  host(i.address) || ' is shown as active, but its latest evidence is from ' || to_char(i.last_seen, 'YYYY-MM-DD') || '.',
	  'Check that discovery still covers this subnet.'
	FROM ip_addresses i LEFT JOIN observation_sources os ON os.id = i.discovery_source_id
	WHERE i.observation_state = 'observed_active' AND i.last_seen < now() - interval '7 days'`},

	{"unknown_ip", `
	SELECT md5('unknown' || o.routing_domain_id || host(o.address)), 'unknown_ip', 'address', NULL::uuid, o.routing_domain_id, o.address,
	  array_agg(DISTINCT os.name), min(o.observed_at), max(o.observed_at), max(os.reliability), max(os.reliability),
	  host(o.address) || ' was observed but has no IP record.', 'Review the evidence, then add a record if it is legitimate.'
	FROM discovery_observations o JOIN observation_sources os ON os.id = o.source_id
	WHERE o.address IS NOT NULL AND o.routing_domain_id IS NOT NULL AND o.observed_at > now() - interval '7 days'
	  AND COALESCE(o.payload->>'stale', 'false') <> 'true'
	  AND NOT EXISTS (SELECT 1 FROM ip_addresses i WHERE i.routing_domain_id = o.routing_domain_id AND i.address = o.address)
	GROUP BY o.routing_domain_id, o.address`},

	{"conflict_suspected", `
	SELECT md5('conflict' || o.routing_domain_id || host(o.address)), 'ip_conflict_suspected', 'address', NULL::uuid, o.routing_domain_id, o.address,
	  array_agg(DISTINCT os.name), min(o.observed_at), max(o.observed_at), max(os.reliability), 40,
	  host(o.address) || ' was seen with ' || count(DISTINCT o.mac) || ' MAC addresses in one day. This alone is not proof of a conflict.',
	  'Rule out HA pairs, virtual IPs and load balancers before treating this as a conflict.'
	FROM discovery_observations o JOIN observation_sources os ON os.id = o.source_id
	WHERE o.address IS NOT NULL AND o.mac IS NOT NULL AND o.routing_domain_id IS NOT NULL AND o.observed_at > now() - interval '1 day'
	  AND COALESCE(o.payload->>'stale', 'false') <> 'true'
	GROUP BY o.routing_domain_id, o.address HAVING count(DISTINCT o.mac) > 1`},

	{"duplicate_mac", `
	SELECT md5('mac' || i.routing_domain_id || i.mac), 'duplicate_mac', 'mac_address', NULL::uuid, i.routing_domain_id, NULL::inet, ARRAY['Inventory'],
	  min(i.created_at), now(), 90, 40,
	  'MAC ' || i.mac || ' is on ' || count(*) || ' addresses: ' || string_agg(host(i.address), ', ' ORDER BY i.address) || '.',
	  'Check whether this is a multi-homed host, HA pair or stale record.'
	FROM ip_addresses i WHERE i.mac IS NOT NULL GROUP BY i.routing_domain_id, i.mac HAVING count(*) > 1`},

	{"dns_mismatch", `
	SELECT md5('dns' || f.id), 'dns_mismatch', 'dns_record', f.id, f.routing_domain_id, f.value::inet, ARRAY['DNS'], f.last_seen, f.last_seen, 50, 60,
	  f.name || ' points to ' || f.value || ', but its PTR record says ' || p.value || '.', 'Correct the forward or reverse record.'
	FROM dns_records f JOIN dns_records p ON p.name = f.value AND p.record_type = 'PTR'
	WHERE f.record_type IN ('A', 'AAAA') AND lower(trim(trailing '.' FROM p.value)) <> lower(trim(trailing '.' FROM f.name))`},

	{"vlan_site_mismatch", `
	SELECT md5('vlan-site' || s.id), 'infrastructure_mismatch', 'subnet', s.id, s.routing_domain_id, NULL::inet, ARRAY['Inventory'], s.updated_at, now(), 100, 90,
	  s.cidr || ' is mapped to a VLAN at a different site.', 'Correct the subnet''s site or VLAN.'
	FROM subnets s JOIN vlans v ON v.id = s.vlan_id
	WHERE s.archived_at IS NULL AND s.site_id IS NOT NULL AND v.site_id IS NOT NULL AND s.site_id <> v.site_id`},

	{"interface_device_mismatch", `
	SELECT md5('iface' || i.id), 'infrastructure_mismatch', 'ip_address', i.id, i.routing_domain_id, i.address, ARRAY['Inventory'], i.updated_at, now(), 100, 90,
	  host(i.address) || ' links to an interface on a different device.', 'Correct the device or interface on this address.'
	FROM ip_addresses i JOIN device_interfaces d ON d.id = i.interface_id
	WHERE i.device_id IS NOT NULL AND d.device_id <> i.device_id`},

	{"cloud_space_missing", `
	SELECT md5('cloud-space' || n.id || sp.space), 'infrastructure_mismatch', 'cloud_network', n.id, n.routing_domain_id, NULL::inet, ARRAY[initcap(a.provider)],
	  n.last_seen, n.last_seen, 90, 80,
	  sp.space || ' on ' || CASE WHEN n.name = '' THEN n.external_id ELSE n.name END || ' has no subnet in inventory.',
	  'Add the subnets of this network to inventory, or confirm it is out of scope.'
	FROM cloud_networks n JOIN cloud_accounts a ON a.id = n.cloud_account_id CROSS JOIN LATERAL unnest(n.address_spaces) AS sp(space)
	WHERE n.routing_domain_id IS NOT NULL AND n.last_seen > now() - interval '7 days' AND NOT EXISTS (
	  SELECT 1 FROM subnets s WHERE s.routing_domain_id = n.routing_domain_id AND s.archived_at IS NULL AND s.cidr && sp.space)`},

	{"new_device", `
	SELECT md5('device' || d.id), 'new_device', 'device', d.id, NULL::uuid, d.management_ip, ARRAY[d.source_system], d.first_discovered, d.last_discovered, 70, 70,
	  'New device ' || d.name || ' was reported by ' || d.source_system || ' and has no owner.', 'Assign an owner and business service.'
	FROM devices d WHERE d.source_system <> 'manual' AND d.owner_id IS NULL AND d.first_discovered > now() - interval '30 days'`},
}
