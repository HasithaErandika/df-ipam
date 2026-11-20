-- Network Links: the company's connectivity (internet lines, broadband, VPN
-- tunnels, MPLS, leased lines, 4G/5G, cloud connections, third parties), kept
-- by hand. Administrative records only: nothing here is polled or configured.
-- Link types, sites and reminder days are lists in settings (links.*).

CREATE FUNCTION all_ipv4(nets cidr[]) RETURNS boolean LANGUAGE sql IMMUTABLE AS $$
  SELECT COALESCE(bool_and(family(n) = 4), true) FROM unnest(nets) n
$$;

CREATE TABLE network_link_providers (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name            text NOT NULL CHECK (btrim(name) <> '' AND length(name) <= 120),
  account_manager text NOT NULL DEFAULT '' CHECK (length(account_manager) <= 120),
  contact_number  text NOT NULL DEFAULT '' CHECK (length(contact_number) <= 40),
  email           text NOT NULL DEFAULT '' CHECK (length(email) <= 254),
  support_hotline text NOT NULL DEFAULT '' CHECK (length(support_hotline) <= 40),
  support_email   text NOT NULL DEFAULT '' CHECK (length(support_email) <= 254),
  website         text NOT NULL DEFAULT '' CHECK (length(website) <= 300),
  remarks         text NOT NULL DEFAULT '' CHECK (length(remarks) <= 1000),
  is_active       boolean NOT NULL DEFAULT true,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX network_link_providers_name_key ON network_link_providers (lower(name));
CREATE TRIGGER network_link_providers_updated BEFORE UPDATE ON network_link_providers FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE SEQUENCE network_link_no;

CREATE TABLE network_links (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code                text NOT NULL UNIQUE DEFAULT 'DF-LNK-' || lpad(nextval('network_link_no')::text, 4, '0'),
  -- Basic
  name                text NOT NULL CHECK (btrim(name) <> '' AND length(name) <= 160),
  link_type           text NOT NULL CHECK (link_type ~ '^[a-z][a-z0-9_]{1,39}$'),
  provider_id         uuid REFERENCES network_link_providers(id),
  circuit_id          text NOT NULL DEFAULT '' CHECK (length(circuit_id) <= 120),
  source_site         text NOT NULL CHECK (btrim(source_site) <> '' AND length(source_site) <= 120),
  destination         text NOT NULL DEFAULT '' CHECK (length(destination) <= 160),
  purpose             text NOT NULL CHECK (btrim(purpose) <> '' AND length(purpose) <= 500),
  environment         text NOT NULL DEFAULT '' CHECK (environment IN ('', 'production', 'dr', 'uat', 'other')),
  link_role           text NOT NULL CHECK (link_role IN ('primary', 'secondary', 'backup', 'standalone')),
  criticality         text NOT NULL CHECK (criticality IN ('critical', 'high', 'medium', 'low')),
  officer_id          uuid REFERENCES users(id),
  status              text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'pending', 'terminated')),
  -- Technical (bandwidth in Mbps)
  down_mbps           numeric(12,3) CHECK (down_mbps > 0),
  up_mbps             numeric(12,3) CHECK (up_mbps > 0),
  medium              text NOT NULL DEFAULT '' CHECK (medium IN ('', 'fibre', 'wireless', 'copper', 'other')),
  wan_ip              inet CHECK (family(wan_ip) = 4 AND masklen(wan_ip) = 32),
  wan_segment_id      uuid REFERENCES segments(id),
  public_ips          cidr CHECK (family(public_ips) = 4),
  gateway_ip          inet CHECK (family(gateway_ip) = 4 AND masklen(gateway_ip) = 32),
  device_id           uuid REFERENCES devices(id),
  device_text         text NOT NULL DEFAULT '' CHECK (length(device_text) <= 120),
  interface           text NOT NULL DEFAULT '' CHECK (length(interface) <= 80),
  vlan                integer CHECK (vlan BETWEEN 1 AND 4094),
  vrf                 text NOT NULL DEFAULT '' CHECK (length(vrf) <= 80),
  routing             text NOT NULL DEFAULT '' CHECK (routing IN ('', 'static', 'bgp', 'ospf', 'other')),
  -- VPN. Remote subnets are the far side's networks: never local address space,
  -- and they may overlap other links' remote subnets.
  vpn_tunnel          text NOT NULL DEFAULT '' CHECK (length(vpn_tunnel) <= 120),
  vpn_type            text NOT NULL DEFAULT '' CHECK (vpn_type IN ('', 'ipsec', 'ssl', 'other')),
  local_peer          text NOT NULL DEFAULT '' CHECK (length(local_peer) <= 253),
  remote_peer         text NOT NULL DEFAULT '' CHECK (length(remote_peer) <= 253),
  remote_subnets      cidr[] NOT NULL DEFAULT '{}' CHECK (cardinality(remote_subnets) <= 100 AND all_ipv4(remote_subnets)),
  remote_org          text NOT NULL DEFAULT '' CHECK (length(remote_org) <= 160),
  remote_contact      text NOT NULL DEFAULT '' CHECK (length(remote_contact) <= 300),
  last_test_date      date,
  vault_ref           text NOT NULL DEFAULT '' CHECK (length(vault_ref) <= 200),
  -- Commercial. no_separate_cost marks a link paid for through another (a VPN
  -- over an internet line), so its cost isn't counted twice.
  no_separate_cost    boolean NOT NULL DEFAULT false,
  billing_account     text NOT NULL DEFAULT '' CHECK (length(billing_account) <= 80),
  monthly_rental      numeric(14,2) CHECK (monthly_rental >= 0),
  currency            text NOT NULL DEFAULT 'LKR' CHECK (currency ~ '^[A-Z]{3}$'),
  install_cost        numeric(14,2) CHECK (install_cost >= 0),
  contract_ref        text NOT NULL DEFAULT '' CHECK (length(contract_ref) <= 120),
  po_ref              text NOT NULL DEFAULT '' CHECK (length(po_ref) <= 120),
  activation_date     date,
  contract_start      date,
  contract_end        date CHECK (contract_end >= contract_start),
  renewal_notice_days integer CHECK (renewal_notice_days BETWEEN 0 AND 730),
  cost_centre         text NOT NULL DEFAULT '' CHECK (length(cost_centre) <= 80),
  -- Which reminder days (90, 60…) were sent for the current contract end.
  reminders_sent      integer[] NOT NULL DEFAULT '{}',
  -- Support (link-specific; the provider's own details are the fallback)
  support_contact     text NOT NULL DEFAULT '' CHECK (length(support_contact) <= 120),
  support_email       text NOT NULL DEFAULT '' CHECK (length(support_email) <= 254),
  support_hotline     text NOT NULL DEFAULT '' CHECK (length(support_hotline) <= 40),
  account_manager     text NOT NULL DEFAULT '' CHECK (length(account_manager) <= 120),
  sla_pct             numeric(6,3) CHECK (sla_pct BETWEEN 0 AND 100),
  resolution_hours    numeric(7,1) CHECK (resolution_hours >= 0),
  remarks             text NOT NULL DEFAULT '' CHECK (length(remarks) <= 4000),
  archived_at         timestamptz,
  archived_by         uuid REFERENCES users(id),
  created_by          uuid REFERENCES users(id),
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  CHECK (NOT no_separate_cost OR (monthly_rental IS NULL OR monthly_rental = 0))
);
CREATE INDEX network_links_type_idx ON network_links (link_type);
CREATE INDEX network_links_provider_idx ON network_links (provider_id);
CREATE INDEX network_links_wan_segment_idx ON network_links (wan_segment_id);
CREATE INDEX network_links_device_idx ON network_links (device_id);
CREATE INDEX network_links_officer_idx ON network_links (officer_id);
CREATE INDEX network_links_contract_end_idx ON network_links (contract_end) WHERE archived_at IS NULL;
CREATE UNIQUE INDEX network_links_circuit_key ON network_links (provider_id, lower(circuit_id)) WHERE circuit_id <> '' AND archived_at IS NULL;
CREATE TRIGGER network_links_updated BEFORE UPDATE ON network_links FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- How one link stands to another: related_id is link_id's secondary, backup or
-- underlying link (a VPN's internet line). One row per pair.
CREATE TABLE network_link_relations (
  link_id    uuid NOT NULL REFERENCES network_links(id),
  related_id uuid NOT NULL REFERENCES network_links(id),
  kind       text NOT NULL CHECK (kind IN ('secondary', 'backup', 'underlying')),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (link_id, related_id),
  CHECK (link_id <> related_id)
);
CREATE INDEX network_link_relations_related_idx ON network_link_relations (related_id);

-- A VPN's local subnets: segments in the register.
CREATE TABLE network_link_segments (
  link_id    uuid NOT NULL REFERENCES network_links(id),
  segment_id uuid NOT NULL REFERENCES segments(id),
  PRIMARY KEY (link_id, segment_id)
);
CREATE INDEX network_link_segments_segment_idx ON network_link_segments (segment_id);

-- Files kept on the server's attachment volume under stored_name. A removed
-- file is marked, not deleted, so the history stays.
CREATE TABLE network_link_attachments (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  link_id      uuid NOT NULL REFERENCES network_links(id),
  category     text NOT NULL CHECK (category IN ('contract', 'quotation', 'diagram', 'configuration', 'other')),
  file_name    text NOT NULL CHECK (btrim(file_name) <> '' AND length(file_name) <= 200),
  content_type text NOT NULL,
  size_bytes   bigint NOT NULL CHECK (size_bytes > 0),
  sha256       text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
  stored_name  text NOT NULL UNIQUE CHECK (stored_name ~ '^[0-9a-f-]{36}$'),
  uploaded_by  uuid REFERENCES users(id),
  uploaded_at  timestamptz NOT NULL DEFAULT now(),
  removed_at   timestamptz,
  removed_by   uuid REFERENCES users(id)
);
CREATE INDEX network_link_attachments_link_idx ON network_link_attachments (link_id, uploaded_at DESC);

ALTER TABLE network_link_providers ENABLE ROW LEVEL SECURITY;
ALTER TABLE network_links ENABLE ROW LEVEL SECURITY;
ALTER TABLE network_link_relations ENABLE ROW LEVEL SECURITY;
ALTER TABLE network_link_segments ENABLE ROW LEVEL SECURITY;
ALTER TABLE network_link_attachments ENABLE ROW LEVEL SECURITY;
CREATE POLICY dfipam_app_all ON network_link_providers TO dfipam_app USING (true) WITH CHECK (true);
CREATE POLICY dfipam_app_all ON network_links TO dfipam_app USING (true) WITH CHECK (true);
CREATE POLICY dfipam_app_all ON network_link_relations TO dfipam_app USING (true) WITH CHECK (true);
CREATE POLICY dfipam_app_all ON network_link_segments TO dfipam_app USING (true) WITH CHECK (true);
CREATE POLICY dfipam_app_all ON network_link_attachments TO dfipam_app USING (true) WITH CHECK (true);

-- Links and providers are archived or deactivated, never deleted; relations
-- and local subnets are replaced as a whole when a link is saved.
GRANT SELECT, INSERT, UPDATE ON network_link_providers, network_links, network_link_attachments TO dfipam_app;
GRANT SELECT, INSERT, DELETE ON network_link_relations, network_link_segments TO dfipam_app;
GRANT USAGE ON SEQUENCE network_link_no TO dfipam_app;
DO $$
DECLARE r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['anon', 'authenticated'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r) THEN
      EXECUTE format('REVOKE ALL ON network_link_providers, network_links, network_link_relations, network_link_segments, network_link_attachments FROM %I', r);
    END IF;
  END LOOP;
END $$;

-- The master lists, editable in Network setup > Network links.
INSERT INTO settings (key, value) VALUES
  ('links.types', '[
    {"key":"ill","label":"Internet Leased Line (ILL)","vpn":false,"active":true},
    {"key":"broadband","label":"Broadband / Fibre","vpn":false,"active":true},
    {"key":"site_vpn","label":"Site-to-Site VPN","vpn":true,"active":true},
    {"key":"remote_vpn","label":"Remote Access VPN","vpn":true,"active":true},
    {"key":"mpls","label":"MPLS / IPVPN","vpn":false,"active":true},
    {"key":"leased_line","label":"Dedicated Leased Line","vpn":false,"active":true},
    {"key":"mobile","label":"4G / 5G","vpn":false,"active":true},
    {"key":"aws_dx","label":"AWS Direct Connect","vpn":false,"active":true},
    {"key":"azure_er","label":"Azure ExpressRoute","vpn":false,"active":true},
    {"key":"third_party","label":"Third-party connectivity","vpn":false,"active":true},
    {"key":"other","label":"Other","vpn":false,"active":true}]'),
  ('links.sites', '["Head Office"]'),
  ('links.reminder_days', '[90, 60, 30, 7, 0]'),
  ('links.usd_rate', 'null')
ON CONFLICT (key) DO NOTHING;

-- The starting roles get the new permissions that fit them. Admin has full access.
UPDATE roles SET permissions = permissions || '{links:view,links:create,links:edit,links:archive,links:commercial_view,links:commercial_manage,links:providers,links:import_export}'
  WHERE key = 'network_engineer';
UPDATE roles SET permissions = permissions || '{links:view}' WHERE key IN ('it_officer', 'viewer', 'requester');
UPDATE roles SET permissions = permissions || '{links:view,links:commercial_view}' WHERE key = 'security_auditor';
