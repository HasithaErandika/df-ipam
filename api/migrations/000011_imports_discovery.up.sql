-- Phase 3 and 4: imports know their target, and discovery jobs remember the
-- schedule or person that started them. No new tables.
ALTER TABLE import_batches
  ADD COLUMN routing_domain_id uuid REFERENCES routing_domains(id),
  ADD COLUMN environment text,
  ADD COLUMN committed_at timestamptz,
  ADD COLUMN reversed_by uuid REFERENCES users(id);
CREATE INDEX import_records_status_idx ON import_records (batch_id, status);

ALTER TABLE discovery_jobs
  ADD COLUMN schedule_id uuid REFERENCES discovery_schedules(id),
  ADD COLUMN subnet_id uuid REFERENCES subnets(id),
  ADD COLUMN claimed_at timestamptz;
ALTER TABLE discovery_schedules ADD COLUMN last_run_at timestamptz;
ALTER TABLE discovery_policies ADD COLUMN description text NOT NULL DEFAULT '';
CREATE INDEX discovery_job_logs_job_idx ON discovery_job_logs (job_id, logged_at);
CREATE INDEX discovery_observations_job_idx ON discovery_observations (job_id);
