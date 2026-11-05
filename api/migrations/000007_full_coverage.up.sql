-- Tables needed by requirements the earlier migrations did not store.
CREATE TABLE sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash text NOT NULL UNIQUE,
  source_ip inet, user_agent text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz
);
COMMENT ON TABLE sessions IS 'Unused while Supabase manages sessions. Reserved for company auth.';

ALTER TABLE subnets ADD COLUMN business_unit_id uuid REFERENCES business_units(id);

CREATE TABLE subnet_reserved_ranges (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  subnet_id uuid NOT NULL REFERENCES subnets(id) ON DELETE CASCADE,
  range_start inet NOT NULL, range_end inet NOT NULL,
  reason text NOT NULL DEFAULT '',
  provider text NOT NULL DEFAULT 'manual' CHECK (provider IN ('manual','aws','azure')),
  CHECK (range_start <= range_end)
);

CREATE TABLE routing_domain_peerings (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  domain_a_id uuid NOT NULL REFERENCES routing_domains(id),
  domain_b_id uuid NOT NULL REFERENCES routing_domains(id),
  kind text NOT NULL DEFAULT 'peering',
  source_id uuid REFERENCES observation_sources(id),
  last_seen timestamptz,
  CHECK (domain_a_id < domain_b_id),
  UNIQUE (domain_a_id, domain_b_id, kind)
);

CREATE TABLE cloud_accounts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  connection_id uuid NOT NULL REFERENCES integration_connections(id),
  provider text NOT NULL CHECK (provider IN ('aws','azure')),
  external_account_id text NOT NULL,
  tenant_id text, name text NOT NULL DEFAULT '',
  last_seen timestamptz,
  UNIQUE (connection_id, external_account_id)
);

CREATE TABLE cloud_networks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  cloud_account_id uuid NOT NULL REFERENCES cloud_accounts(id),
  routing_domain_id uuid REFERENCES routing_domains(id),
  region text NOT NULL DEFAULT '', resource_group text,
  external_id text NOT NULL, name text NOT NULL DEFAULT '',
  address_spaces cidr[] NOT NULL DEFAULT '{}',
  last_seen timestamptz,
  UNIQUE (cloud_account_id, external_id)
);

CREATE TABLE network_routes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  routing_domain_id uuid REFERENCES routing_domains(id),
  device_id uuid REFERENCES devices(id),
  vdom_id uuid REFERENCES vdoms(id),
  destination cidr NOT NULL, next_hop inet,
  interface_id uuid REFERENCES device_interfaces(id),
  route_type text NOT NULL DEFAULT 'static',
  source_id uuid REFERENCES observation_sources(id),
  last_seen timestamptz
);
CREATE INDEX network_routes_destination_gist ON network_routes USING gist (destination inet_ops);

CREATE TABLE firewall_address_objects (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  device_id uuid NOT NULL REFERENCES devices(id),
  vdom_id uuid REFERENCES vdoms(id),
  name text NOT NULL, object_type text NOT NULL DEFAULT '',
  value text NOT NULL DEFAULT '', network cidr, comment text NOT NULL DEFAULT '',
  last_seen timestamptz,
  UNIQUE NULLS NOT DISTINCT (device_id, vdom_id, name)
);

CREATE TABLE firewall_address_groups (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  device_id uuid NOT NULL REFERENCES devices(id),
  vdom_id uuid REFERENCES vdoms(id),
  name text NOT NULL, comment text NOT NULL DEFAULT '',
  last_seen timestamptz,
  UNIQUE NULLS NOT DISTINCT (device_id, vdom_id, name)
);

CREATE TABLE firewall_address_group_members (
  group_id uuid NOT NULL REFERENCES firewall_address_groups(id) ON DELETE CASCADE,
  object_id uuid NOT NULL REFERENCES firewall_address_objects(id) ON DELETE CASCADE,
  PRIMARY KEY (group_id, object_id)
);

CREATE TABLE agent_registration_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  agent_id uuid REFERENCES discovery_agents(id),
  token_hash text NOT NULL UNIQUE,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  used_at timestamptz
);

CREATE TABLE discovery_blackouts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL,
  target cidr,
  starts_at timestamptz NOT NULL, ends_at timestamptz NOT NULL,
  reason text NOT NULL DEFAULT '',
  created_by uuid REFERENCES users(id),
  CHECK (starts_at < ends_at)
);

CREATE TABLE reconciliation_rules (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  classification text NOT NULL UNIQUE,
  source_priority jsonb NOT NULL DEFAULT '[]',
  confidence_weights jsonb NOT NULL DEFAULT '{}',
  is_enabled boolean NOT NULL DEFAULT true
);

CREATE TABLE notification_preferences (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  event_type text NOT NULL,
  channel text NOT NULL DEFAULT 'email',
  is_enabled boolean NOT NULL DEFAULT true,
  PRIMARY KEY (user_id, event_type, channel)
);

CREATE TABLE retention_policies (
  data_class text PRIMARY KEY,
  retain_days integer NOT NULL CHECK (retain_days > 0),
  archive_first boolean NOT NULL DEFAULT false,
  updated_at timestamptz NOT NULL DEFAULT now(),
  updated_by uuid REFERENCES users(id)
);

-- Foreign keys that list pages filter on.
CREATE INDEX subnets_site_idx ON subnets (site_id);
CREATE INDEX subnets_vlan_idx ON subnets (vlan_id);
CREATE INDEX subnets_vdom_idx ON subnets (vdom_id);
CREATE INDEX subnets_environment_idx ON subnets (environment);
CREATE INDEX subnets_business_service_idx ON subnets (business_service_id);
CREATE INDEX ip_addresses_device_idx ON ip_addresses (device_id);
CREATE INDEX ip_addresses_owner_idx ON ip_addresses (owner_id);
CREATE INDEX device_interfaces_mac_idx ON device_interfaces (mac);
CREATE INDEX devices_site_idx ON devices (site_id);
CREATE INDEX ip_requests_status_idx ON ip_requests (status);
CREATE INDEX ip_requests_requester_idx ON ip_requests (requested_by);
CREATE INDEX ip_reservations_expiry_idx ON ip_reservations (expires_at) WHERE status = 'active';

-- New tables get the same protection as the others: closed to the Data API,
-- least privilege for the app role.
DO $$
DECLARE
  t text;
  r text;
  new_tables text[] := ARRAY['sessions','subnet_reserved_ranges','routing_domain_peerings','cloud_accounts',
    'cloud_networks','network_routes','firewall_address_objects','firewall_address_groups',
    'firewall_address_group_members','agent_registration_tokens','discovery_blackouts','reconciliation_rules',
    'notification_preferences','retention_policies'];
BEGIN
  FOREACH t IN ARRAY new_tables LOOP
    EXECUTE format('ALTER TABLE public.%I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('CREATE POLICY dfipam_app_all ON public.%I TO dfipam_app USING (true) WITH CHECK (true)', t);
    EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON public.%I TO dfipam_app', t);
    FOREACH r IN ARRAY ARRAY['anon', 'authenticated'] LOOP
      IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r) THEN
        EXECUTE format('REVOKE ALL ON public.%I FROM %I', t, r);
      END IF;
    END LOOP;
  END LOOP;
END $$;

CREATE TRIGGER retention_policies_updated BEFORE UPDATE ON retention_policies
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
