CREATE TABLE approval_workflows (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name text NOT NULL UNIQUE, match_rules jsonb NOT NULL DEFAULT '{}', is_active boolean NOT NULL DEFAULT true
);

CREATE TABLE approval_workflow_steps (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  workflow_id uuid NOT NULL REFERENCES approval_workflows(id) ON DELETE CASCADE,
  step_order smallint NOT NULL,
  approver_role_id uuid NOT NULL REFERENCES roles(id),
  UNIQUE (workflow_id, step_order)
);

CREATE TABLE ip_requests (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  request_type text NOT NULL DEFAULT 'allocation' CHECK (request_type IN ('allocation','reservation','release')),
  subnet_id uuid NOT NULL REFERENCES subnets(id),
  requested_address inet,
  selection_mode text NOT NULL DEFAULT 'automatic' CHECK (selection_mode IN ('automatic','specific')),
  application_id uuid REFERENCES applications(id),
  business_service_id uuid REFERENCES business_services(id),
  justification text NOT NULL DEFAULT '', department text NOT NULL DEFAULT '',
  technical_owner_id uuid REFERENCES users(id), business_owner_id uuid REFERENCES users(id),
  environment text CHECK (environment IN ('production','disaster_recovery','uat','development','testing')),
  required_date date, expiry_date date, comments text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'draft' CHECK (status IN
    ('draft','submitted','pending_approval','approved','rejected','allocated','cancelled','expired')),
  requested_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (selection_mode = 'automatic' OR requested_address IS NOT NULL)
);

CREATE TABLE ip_request_approvals (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  request_id uuid NOT NULL REFERENCES ip_requests(id),
  step_order smallint NOT NULL,
  approver_id uuid REFERENCES users(id),
  decision text NOT NULL DEFAULT 'pending' CHECK (decision IN ('pending','approved','rejected')),
  comment text NOT NULL DEFAULT '', decided_at timestamptz,
  UNIQUE (request_id, step_order)
);

CREATE TABLE ip_allocations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  ip_address_id uuid NOT NULL, subnet_id uuid NOT NULL, routing_domain_id uuid NOT NULL, address inet NOT NULL,
  request_id uuid REFERENCES ip_requests(id),
  release_request_id uuid REFERENCES ip_requests(id),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','released')),
  allocated_by uuid NOT NULL REFERENCES users(id), allocated_at timestamptz NOT NULL DEFAULT now(),
  released_by uuid REFERENCES users(id), released_at timestamptz,
  FOREIGN KEY (ip_address_id, subnet_id, routing_domain_id, address)
    REFERENCES ip_addresses (id, subnet_id, routing_domain_id, address)
);
CREATE UNIQUE INDEX ip_alloc_one_active ON ip_allocations (routing_domain_id, address) WHERE status = 'active';

CREATE TABLE ip_reservations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  ip_address_id uuid NOT NULL, subnet_id uuid NOT NULL, routing_domain_id uuid NOT NULL, address inet NOT NULL,
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','released','expired','converted')),
  reason text NOT NULL DEFAULT '', expires_at timestamptz,
  reserved_by uuid NOT NULL REFERENCES users(id), reserved_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (ip_address_id, subnet_id, routing_domain_id, address)
    REFERENCES ip_addresses (id, subnet_id, routing_domain_id, address)
);
CREATE UNIQUE INDEX ip_reservation_one_active ON ip_reservations (routing_domain_id, address) WHERE status = 'active';

CREATE TABLE change_requests (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  object_type text NOT NULL, object_id uuid,
  proposed jsonb NOT NULL,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected','applied')),
  requested_by uuid REFERENCES users(id), decided_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(), decided_at timestamptz
);

CREATE TABLE audit_logs (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  actor_id uuid REFERENCES users(id),
  actor_type text NOT NULL DEFAULT 'user' CHECK (actor_type IN ('user','system','agent')),
  action text NOT NULL,
  object_type text NOT NULL, object_id text,
  old_values jsonb, new_values jsonb,
  occurred_at timestamptz NOT NULL DEFAULT now(),
  source_ip inet, correlation_id text, approval_ref text,
  result text NOT NULL DEFAULT 'success' CHECK (result IN ('success','failure','denied'))
);
CREATE INDEX audit_logs_object_idx ON audit_logs (object_type, object_id, occurred_at DESC);
CREATE INDEX audit_logs_time_idx ON audit_logs (occurred_at DESC);

CREATE TABLE notification_rules (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  event_type text NOT NULL UNIQUE,
  severity text NOT NULL DEFAULT 'info' CHECK (severity IN ('info','warning','critical')),
  recipient_group text NOT NULL DEFAULT '', is_enabled boolean NOT NULL DEFAULT true
);

CREATE TABLE notifications (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  event_type text NOT NULL,
  severity text NOT NULL DEFAULT 'info' CHECK (severity IN ('info','warning','critical')),
  recipient_id uuid REFERENCES users(id),
  channel text NOT NULL DEFAULT 'in_app', dedupe_key text,
  payload jsonb NOT NULL DEFAULT '{}',
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','sent','failed','suppressed')),
  attempts smallint NOT NULL DEFAULT 0,
  sent_at timestamptz, read_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notifications_recipient_idx ON notifications (recipient_id, created_at DESC);

CREATE TABLE report_jobs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  report_type text NOT NULL,
  format text NOT NULL CHECK (format IN ('xlsx','csv','pdf')),
  filters jsonb NOT NULL DEFAULT '{}',
  status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','completed','failed')),
  file_path text, data_as_of timestamptz,
  requested_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE import_batches (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  file_name text NOT NULL, file_hash text NOT NULL UNIQUE, format text NOT NULL,
  status text NOT NULL DEFAULT 'uploaded' CHECK (status IN
    ('uploaded','validated','approved','committed','reversed','failed')),
  summary jsonb NOT NULL DEFAULT '{}',
  approved_by uuid REFERENCES users(id), reversed_at timestamptz,
  uploaded_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE import_records (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  batch_id uuid NOT NULL REFERENCES import_batches(id),
  line_number integer NOT NULL, raw_text text NOT NULL,
  parsed jsonb, status text NOT NULL DEFAULT 'pending' CHECK (status IN
    ('pending','valid','duplicate','overlap','invalid','needs_enrichment','committed')),
  errors jsonb NOT NULL DEFAULT '[]',
  created_object_type text, created_object_id uuid,
  UNIQUE (batch_id, line_number)
);

CREATE TABLE saved_filters (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  page text NOT NULL, name text NOT NULL, filters jsonb NOT NULL,
  UNIQUE (user_id, page, name)
);
