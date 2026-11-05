CREATE TABLE organisations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL UNIQUE,
  code text NOT NULL UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  created_by uuid REFERENCES users(id), updated_by uuid REFERENCES users(id)
);

CREATE TABLE business_units (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organisation_id uuid NOT NULL REFERENCES organisations(id),
  name text NOT NULL, code text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  created_by uuid REFERENCES users(id), updated_by uuid REFERENCES users(id),
  UNIQUE (organisation_id, code)
);

CREATE TABLE sites (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organisation_id uuid NOT NULL REFERENCES organisations(id),
  name text NOT NULL, code text NOT NULL,
  site_type text NOT NULL DEFAULT 'site' CHECK (site_type IN ('site','data_centre','cloud_region','branch')),
  address text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  created_by uuid REFERENCES users(id), updated_by uuid REFERENCES users(id),
  UNIQUE (organisation_id, code)
);

CREATE TABLE network_zones (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  site_id uuid REFERENCES sites(id),
  name text NOT NULL, security_zone text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  created_by uuid REFERENCES users(id), updated_by uuid REFERENCES users(id)
);

CREATE TABLE routing_domains (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organisation_id uuid NOT NULL REFERENCES organisations(id),
  name text NOT NULL,
  kind text NOT NULL DEFAULT 'default' CHECK (kind IN ('default','vrf','vpc','vnet','isolated')),
  route_distinguisher text,
  is_isolated boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  created_by uuid REFERENCES users(id), updated_by uuid REFERENCES users(id),
  UNIQUE (organisation_id, name)
);

CREATE TABLE business_services (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL UNIQUE,
  criticality text NOT NULL DEFAULT 'standard' CHECK (criticality IN ('low','standard','high','critical')),
  is_sensitive boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  created_by uuid REFERENCES users(id), updated_by uuid REFERENCES users(id)
);

CREATE TABLE applications (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL UNIQUE,
  business_service_id uuid REFERENCES business_services(id),
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  created_by uuid REFERENCES users(id), updated_by uuid REFERENCES users(id)
);

CREATE TABLE teams (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL UNIQUE, email text NOT NULL DEFAULT '', description text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE team_members (
  team_id uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  PRIMARY KEY (team_id, user_id)
);

-- Scope columns need the tables above, so user_roles lives here.
CREATE TABLE user_roles (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role_id uuid NOT NULL REFERENCES roles(id),
  organisation_id uuid REFERENCES organisations(id),
  business_unit_id uuid REFERENCES business_units(id),
  site_id uuid REFERENCES sites(id),
  business_service_id uuid REFERENCES business_services(id),
  UNIQUE NULLS NOT DISTINCT (user_id, role_id, organisation_id, business_unit_id, site_id, business_service_id)
);

CREATE TABLE observation_sources (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL UNIQUE,
  kind text NOT NULL,
  reliability smallint NOT NULL DEFAULT 50 CHECK (reliability BETWEEN 0 AND 100),
  priority smallint NOT NULL DEFAULT 50
);

CREATE TABLE overlap_exceptions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  reason text NOT NULL CHECK (length(trim(reason)) > 0),
  approved_by uuid NOT NULL REFERENCES users(id),
  approved_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz
);

CREATE TABLE devices (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL, hostname text NOT NULL DEFAULT '',
  management_ip inet,
  vendor text NOT NULL DEFAULT '', model text NOT NULL DEFAULT '', serial_number text,
  device_type text NOT NULL DEFAULT 'other' CHECK (device_type IN
    ('firewall','router','switch','physical_server','virtual_machine','load_balancer','network_appliance','cloud_interface','other')),
  os_version text NOT NULL DEFAULT '',
  site_id uuid REFERENCES sites(id),
  environment text CHECK (environment IN ('production','disaster_recovery','uat','development','testing')),
  owner_id uuid REFERENCES users(id),
  business_service_id uuid REFERENCES business_services(id),
  source_system text NOT NULL DEFAULT 'manual', external_id text,
  first_discovered timestamptz, last_discovered timestamptz, last_verified timestamptz,
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','inactive','retired','unknown')),
  sensitivity text NOT NULL DEFAULT 'standard' CHECK (sensitivity IN ('standard','sensitive','restricted')),
  notes text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  created_by uuid REFERENCES users(id), updated_by uuid REFERENCES users(id),
  UNIQUE NULLS DISTINCT (source_system, external_id)
);

CREATE TABLE vdoms (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  device_id uuid NOT NULL REFERENCES devices(id),
  routing_domain_id uuid REFERENCES routing_domains(id),
  name text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  created_by uuid REFERENCES users(id), updated_by uuid REFERENCES users(id),
  UNIQUE (device_id, name)
);

CREATE TABLE vlans (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  vlan_id integer NOT NULL CHECK (vlan_id BETWEEN 1 AND 4094),
  name text NOT NULL DEFAULT '',
  site_id uuid REFERENCES sites(id),
  vdom_id uuid REFERENCES vdoms(id),
  network_zone_id uuid REFERENCES network_zones(id),
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  created_by uuid REFERENCES users(id), updated_by uuid REFERENCES users(id),
  UNIQUE NULLS NOT DISTINCT (vlan_id, site_id, vdom_id)
);

CREATE TABLE network_blocks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  routing_domain_id uuid NOT NULL REFERENCES routing_domains(id),
  parent_id uuid REFERENCES network_blocks(id),
  cidr cidr NOT NULL,
  name text NOT NULL, purpose text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  created_by uuid REFERENCES users(id), updated_by uuid REFERENCES users(id),
  UNIQUE (routing_domain_id, cidr)
);
CREATE INDEX network_blocks_cidr_gist ON network_blocks USING gist (cidr inet_ops);

CREATE TABLE subnets (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  routing_domain_id uuid NOT NULL REFERENCES routing_domains(id),
  network_block_id uuid REFERENCES network_blocks(id),
  cidr cidr NOT NULL,
  name text NOT NULL CHECK (length(trim(name)) > 0),
  gateway inet,
  site_id uuid REFERENCES sites(id),
  network_zone_id uuid REFERENCES network_zones(id),
  vdom_id uuid REFERENCES vdoms(id),
  vlan_id uuid REFERENCES vlans(id),
  environment text NOT NULL CHECK (environment IN ('production','disaster_recovery','uat','development','testing')),
  security_zone text NOT NULL DEFAULT '',
  business_service_id uuid REFERENCES business_services(id),
  technical_owner_id uuid REFERENCES users(id),
  business_owner_id uuid REFERENCES users(id),
  description text NOT NULL DEFAULT '',
  discovery_enabled boolean NOT NULL DEFAULT false,
  discovery_policy_id uuid,
  discovery_schedule_id uuid,
  warn_threshold smallint NOT NULL DEFAULT 80,
  critical_threshold smallint NOT NULL DEFAULT 95,
  sensitivity text NOT NULL DEFAULT 'standard' CHECK (sensitivity IN ('standard','sensitive','restricted')),
  overlap_exception_id uuid REFERENCES overlap_exceptions(id),
  version integer NOT NULL DEFAULT 1,
  archived_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  created_by uuid REFERENCES users(id), updated_by uuid REFERENCES users(id),
  UNIQUE (id, routing_domain_id),
  CHECK (gateway IS NULL OR gateway <<= cidr),
  CHECK (warn_threshold BETWEEN 1 AND 100 AND critical_threshold BETWEEN 1 AND 100 AND warn_threshold < critical_threshold),
  CONSTRAINT subnets_no_overlap EXCLUDE USING gist (routing_domain_id WITH =, cidr inet_ops WITH &&)
    WHERE (archived_at IS NULL AND overlap_exception_id IS NULL)
);
CREATE INDEX subnets_active_cidr_gist ON subnets USING gist (routing_domain_id, cidr inet_ops) WHERE archived_at IS NULL;

CREATE TABLE device_interfaces (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  device_id uuid NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
  name text NOT NULL, mac macaddr,
  interface_type text NOT NULL DEFAULT '',
  admin_status text NOT NULL DEFAULT 'unknown', oper_status text NOT NULL DEFAULT 'unknown',
  vlan_id uuid REFERENCES vlans(id),
  vdom_id uuid REFERENCES vdoms(id),
  routing_domain_id uuid REFERENCES routing_domains(id),
  description text NOT NULL DEFAULT '', source text NOT NULL DEFAULT 'manual',
  last_observed timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE NULLS NOT DISTINCT (device_id, vdom_id, name)
);

CREATE TABLE ip_addresses (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  routing_domain_id uuid NOT NULL,
  subnet_id uuid NOT NULL,
  address inet NOT NULL,
  hostname text NOT NULL DEFAULT '', fqdn text NOT NULL DEFAULT '',
  mac macaddr,
  device_id uuid REFERENCES devices(id),
  interface_id uuid REFERENCES device_interfaces(id),
  application_id uuid REFERENCES applications(id),
  business_service_id uuid REFERENCES business_services(id),
  owner_id uuid REFERENCES users(id),
  allocation_state text NOT NULL DEFAULT 'available_candidate' CHECK (allocation_state IN
    ('allocated','reserved','available_candidate','quarantined','retired')),
  observation_state text NOT NULL DEFAULT 'unknown' CHECK (observation_state IN
    ('unknown','observed_active','observed_inactive','conflict_suspected')),
  confidence_score smallint CHECK (confidence_score BETWEEN 0 AND 100),
  confidence_explanation text NOT NULL DEFAULT '',
  discovery_source_id uuid REFERENCES observation_sources(id),
  first_seen timestamptz, last_seen timestamptz, last_verified timestamptz,
  notes text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  created_by uuid REFERENCES users(id), updated_by uuid REFERENCES users(id),
  FOREIGN KEY (subnet_id, routing_domain_id) REFERENCES subnets (id, routing_domain_id),
  UNIQUE (routing_domain_id, address),
  UNIQUE (id, subnet_id, routing_domain_id, address),
  CHECK (masklen(address) = CASE family(address) WHEN 4 THEN 32 ELSE 128 END)
);
CREATE INDEX ip_addresses_subnet_state_idx ON ip_addresses (subnet_id, allocation_state);
CREATE INDEX ip_addresses_mac_idx ON ip_addresses (mac);
CREATE INDEX ip_addresses_hostname_idx ON ip_addresses (lower(hostname));
CREATE INDEX ip_addresses_fqdn_idx ON ip_addresses (lower(fqdn));

CREATE TABLE ownership_assignments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  object_type text NOT NULL, object_id uuid NOT NULL,
  owner_type text NOT NULL CHECK (owner_type IN ('technical','business')),
  user_id uuid REFERENCES users(id), team_id uuid REFERENCES teams(id),
  valid_from timestamptz NOT NULL DEFAULT now(), valid_to timestamptz,
  CHECK (num_nonnulls(user_id, team_id) = 1)
);
CREATE INDEX ownership_object_idx ON ownership_assignments (object_type, object_id);

CREATE TABLE dns_records (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  routing_domain_id uuid REFERENCES routing_domains(id),
  ip_address_id uuid REFERENCES ip_addresses(id),
  name text NOT NULL,
  record_type text NOT NULL CHECK (record_type IN ('A','AAAA','PTR','CNAME')),
  value text NOT NULL, ttl integer,
  source_id uuid REFERENCES observation_sources(id),
  last_seen timestamptz
);
CREATE INDEX dns_records_name_idx ON dns_records (lower(name));

CREATE TABLE dhcp_scopes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  subnet_id uuid NOT NULL REFERENCES subnets(id),
  name text NOT NULL DEFAULT '',
  range_start inet NOT NULL, range_end inet NOT NULL,
  server_device_id uuid REFERENCES devices(id),
  source_id uuid REFERENCES observation_sources(id),
  last_seen timestamptz,
  CHECK (range_start <= range_end)
);

CREATE TABLE dhcp_reservations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  scope_id uuid NOT NULL REFERENCES dhcp_scopes(id) ON DELETE CASCADE,
  address inet NOT NULL, mac macaddr, hostname text NOT NULL DEFAULT '',
  source_id uuid REFERENCES observation_sources(id),
  last_seen timestamptz,
  UNIQUE (scope_id, address)
);

CREATE TABLE subnet_utilisation_snapshots (
  subnet_id uuid NOT NULL REFERENCES subnets(id),
  captured_at timestamptz NOT NULL,
  allocated numeric(39,0) NOT NULL, reserved numeric(39,0) NOT NULL, observed_active numeric(39,0) NOT NULL,
  utilisation_pct numeric(6,3) NOT NULL,
  PRIMARY KEY (subnet_id, captured_at)
);
