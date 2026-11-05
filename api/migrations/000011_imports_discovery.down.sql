DROP INDEX discovery_observations_job_idx, discovery_job_logs_job_idx, import_records_status_idx;
ALTER TABLE discovery_policies DROP COLUMN description;
ALTER TABLE discovery_schedules DROP COLUMN last_run_at;
ALTER TABLE discovery_jobs DROP COLUMN claimed_at, DROP COLUMN subnet_id, DROP COLUMN schedule_id;
ALTER TABLE import_batches DROP COLUMN reversed_by, DROP COLUMN committed_at, DROP COLUMN environment, DROP COLUMN routing_domain_id;
