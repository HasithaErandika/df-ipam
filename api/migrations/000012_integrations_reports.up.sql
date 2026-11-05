-- Phases 5 to 7: connector scheduling, change requests from findings, configurable
-- confidence, generated reports and worker health. No new tables, so the row level
-- security and grants from earlier migrations still cover everything.

-- Connections are scheduled by the worker. Credentials stay write-only in
-- credential_ciphertext, which no API response ever selects.
ALTER TABLE integration_connections
  ADD COLUMN is_enabled boolean NOT NULL DEFAULT true,
  ADD COLUMN sync_interval_minutes integer NOT NULL DEFAULT 60 CHECK (sync_interval_minutes BETWEEN 5 AND 10080),
  ADD COLUMN last_sync_at timestamptz,
  ADD COLUMN last_error text NOT NULL DEFAULT '';

-- contexts lists the VDOMs and networks a sync saw, so a person can map each one
-- to a routing domain. Connectors never guess that mapping.
ALTER TABLE integration_sync_jobs
  ADD COLUMN created_at timestamptz NOT NULL DEFAULT now(),
  ADD COLUMN requested_by uuid REFERENCES users(id),
  ADD COLUMN contexts text[] NOT NULL DEFAULT '{}',
  ADD COLUMN error text NOT NULL DEFAULT '';
-- One sync at a time per connection, whichever worker or person starts it.
CREATE UNIQUE INDEX integration_sync_one_active ON integration_sync_jobs (connection_id) WHERE status IN ('queued', 'running');
CREATE INDEX integration_sync_queue_idx ON integration_sync_jobs (created_at) WHERE status = 'queued';
CREATE INDEX integration_sync_history_idx ON integration_sync_jobs (connection_id, created_at DESC);
CREATE INDEX integration_errors_job_idx ON integration_errors (sync_job_id);
CREATE INDEX discovery_observations_sync_idx ON discovery_observations (sync_job_id) WHERE sync_job_id IS NOT NULL;

-- Change requests carry a finding's proposal through a second person. The check
-- backs up the service: nobody decides their own request.
ALTER TABLE change_requests
  ADD COLUMN finding_id uuid REFERENCES reconciliation_results(id),
  ADD COLUMN reason text NOT NULL DEFAULT '',
  ADD COLUMN decision_note text NOT NULL DEFAULT '',
  ADD COLUMN applied_at timestamptz,
  ADD CONSTRAINT change_requests_no_self_decision CHECK (decided_by IS NULL OR decided_by <> requested_by);
CREATE INDEX change_requests_status_idx ON change_requests (status, created_at DESC);
CREATE UNIQUE INDEX change_requests_one_open_per_finding ON change_requests (finding_id)
  WHERE finding_id IS NOT NULL AND status IN ('pending', 'approved');

-- Each contribution to a finding's confidence, so the screen can explain the score.
ALTER TABLE reconciliation_results ADD COLUMN confidence_breakdown jsonb NOT NULL DEFAULT '{}';

-- Report files live with their job: the API and worker run as separate users and
-- share no directory. Retention removes old rows.
ALTER TABLE report_jobs
  ADD COLUMN content bytea,
  ADD COLUMN content_type text NOT NULL DEFAULT '',
  ADD COLUMN size_bytes bigint NOT NULL DEFAULT 0,
  ADD COLUMN row_count bigint NOT NULL DEFAULT 0,
  ADD COLUMN truncated boolean NOT NULL DEFAULT false,
  ADD COLUMN error text NOT NULL DEFAULT '',
  ADD COLUMN started_at timestamptz,
  ADD COLUMN finished_at timestamptz;
CREATE INDEX report_jobs_queue_idx ON report_jobs (created_at) WHERE status = 'queued';
CREATE INDEX report_jobs_owner_idx ON report_jobs (requested_by, created_at DESC);

CREATE INDEX subnet_utilisation_snapshots_time_idx ON subnet_utilisation_snapshots (captured_at);
CREATE INDEX audit_logs_filter_idx ON audit_logs (action, occurred_at DESC);

-- Once a person edits a role, make seed stops restoring its default permissions, so a
-- permission taken away doesn't quietly come back on the next deployment.
ALTER TABLE roles ADD COLUMN is_customised boolean NOT NULL DEFAULT false;
