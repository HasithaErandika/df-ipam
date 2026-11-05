CREATE TABLE discovery_agents (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL UNIQUE,
  site_id uuid REFERENCES sites(id),
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','active','offline','revoked')),
  credential_hash text, credential_rotated_at timestamptz, revoked_at timestamptz,
  version text NOT NULL DEFAULT '', last_heartbeat_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE discovery_policies (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL UNIQUE,
  methods text[] NOT NULL DEFAULT '{}',
  target_allowlist cidr[] NOT NULL DEFAULT '{}',
  max_concurrency integer NOT NULL DEFAULT 16 CHECK (max_concurrency BETWEEN 1 AND 256),
  max_pps integer NOT NULL DEFAULT 50 CHECK (max_pps BETWEEN 1 AND 1000),
  timeout_ms integer NOT NULL DEFAULT 1000 CHECK (timeout_ms BETWEEN 100 AND 30000),
  retries smallint NOT NULL DEFAULT 1 CHECK (retries BETWEEN 0 AND 5),
  windows jsonb NOT NULL DEFAULT '[]',
  is_sensitive boolean NOT NULL DEFAULT false,
  approved_by uuid REFERENCES users(id), approved_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  created_by uuid REFERENCES users(id), updated_by uuid REFERENCES users(id)
);

CREATE TABLE discovery_schedules (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  policy_id uuid NOT NULL REFERENCES discovery_policies(id),
  cron text NOT NULL, timezone text NOT NULL DEFAULT 'Asia/Colombo',
  subnet_id uuid REFERENCES subnets(id),
  agent_id uuid REFERENCES discovery_agents(id),
  priority smallint NOT NULL DEFAULT 5,
  is_enabled boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE subnets
  ADD FOREIGN KEY (discovery_policy_id) REFERENCES discovery_policies(id),
  ADD FOREIGN KEY (discovery_schedule_id) REFERENCES discovery_schedules(id);

CREATE TABLE discovery_jobs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  policy_id uuid REFERENCES discovery_policies(id),
  agent_id uuid REFERENCES discovery_agents(id),
  status text NOT NULL DEFAULT 'queued' CHECK (status IN
    ('queued','running','paused','completed','failed','cancelled','stopped')),
  priority smallint NOT NULL DEFAULT 5,
  config_snapshot jsonb NOT NULL,
  progress smallint NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
  error text,
  started_at timestamptz, finished_at timestamptz,
  initiated_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX discovery_jobs_status_idx ON discovery_jobs (status, priority);

CREATE TABLE discovery_job_targets (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  job_id uuid NOT NULL REFERENCES discovery_jobs(id),
  routing_domain_id uuid NOT NULL REFERENCES routing_domains(id),
  target cidr NOT NULL,
  checkpoint text,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','done','failed','skipped'))
);

CREATE TABLE discovery_job_logs (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  job_id uuid NOT NULL REFERENCES discovery_jobs(id),
  level text NOT NULL, message text NOT NULL,
  logged_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE discovery_exclusions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  target cidr NOT NULL,
  routing_domain_id uuid REFERENCES routing_domains(id),
  reason text NOT NULL, expires_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  created_by uuid REFERENCES users(id)
);
CREATE INDEX discovery_exclusions_target_gist ON discovery_exclusions USING gist (target inet_ops);

CREATE TABLE agent_heartbeats (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  agent_id uuid NOT NULL REFERENCES discovery_agents(id),
  received_at timestamptz NOT NULL DEFAULT now(),
  version text NOT NULL DEFAULT '', metrics jsonb NOT NULL DEFAULT '{}'
);
CREATE INDEX agent_heartbeats_idx ON agent_heartbeats (agent_id, received_at DESC);

CREATE TABLE integration_connections (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  kind text NOT NULL CHECK (kind IN ('fortigate','vcenter','aws','azure','dns','dhcp')),
  name text NOT NULL UNIQUE,
  base_url text, config jsonb NOT NULL DEFAULT '{}',
  credential_ciphertext bytea, key_version integer,
  health text NOT NULL DEFAULT 'unknown' CHECK (health IN ('unknown','healthy','degraded','failed')),
  last_success_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  created_by uuid REFERENCES users(id), updated_by uuid REFERENCES users(id)
);

CREATE TABLE integration_sync_jobs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  connection_id uuid NOT NULL REFERENCES integration_connections(id),
  status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','completed','failed','cancelled')),
  counts jsonb NOT NULL DEFAULT '{}',
  started_at timestamptz, finished_at timestamptz
);

CREATE TABLE external_resource_mappings (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  connection_id uuid NOT NULL REFERENCES integration_connections(id),
  resource_type text NOT NULL, external_id text NOT NULL,
  object_type text NOT NULL, object_id uuid NOT NULL,
  last_seen timestamptz,
  UNIQUE (connection_id, resource_type, external_id)
);

CREATE TABLE integration_errors (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  sync_job_id uuid NOT NULL REFERENCES integration_sync_jobs(id),
  code text NOT NULL, message text NOT NULL,
  context jsonb NOT NULL DEFAULT '{}',
  occurred_at timestamptz NOT NULL DEFAULT now()
);

-- Raw evidence: separate from inventory and append-only for the app role.
CREATE TABLE discovery_observations (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  job_id uuid REFERENCES discovery_jobs(id),
  sync_job_id uuid REFERENCES integration_sync_jobs(id),
  source_id uuid NOT NULL REFERENCES observation_sources(id),
  routing_domain_id uuid REFERENCES routing_domains(id),
  address inet, mac macaddr, hostname text,
  payload jsonb NOT NULL DEFAULT '{}',
  observed_at timestamptz NOT NULL
);
CREATE INDEX discovery_observations_addr_idx ON discovery_observations (address, observed_at DESC);

CREATE TABLE reconciliation_results (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  classification text NOT NULL CHECK (classification IN
    ('new_device','unknown_ip','ip_conflict_suspected','duplicate_mac','dns_mismatch','allocated_not_observed',
     'observed_not_allocated','subnet_overlap','ownership_missing','infrastructure_mismatch','stale_evidence')),
  object_type text NOT NULL, object_id uuid,
  routing_domain_id uuid REFERENCES routing_domains(id),
  address inet,
  sources text[] NOT NULL DEFAULT '{}',
  evidence_from timestamptz, evidence_to timestamptz,
  source_reliability smallint, confidence smallint CHECK (confidence BETWEEN 0 AND 100),
  explanation text NOT NULL, suggested_action text NOT NULL DEFAULT '',
  review_status text NOT NULL DEFAULT 'open' CHECK (review_status IN ('open','investigating','accepted','dismissed','resolved')),
  reviewed_by uuid REFERENCES users(id), reviewed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX reconciliation_review_idx ON reconciliation_results (review_status, classification);

CREATE TABLE reconciliation_evidence (
  result_id uuid NOT NULL REFERENCES reconciliation_results(id) ON DELETE CASCADE,
  observation_id bigint NOT NULL REFERENCES discovery_observations(id),
  PRIMARY KEY (result_id, observation_id)
);
