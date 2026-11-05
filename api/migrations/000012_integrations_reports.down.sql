ALTER TABLE roles DROP COLUMN is_customised;
DROP INDEX audit_logs_filter_idx;
DROP INDEX subnet_utilisation_snapshots_time_idx;
DROP INDEX report_jobs_owner_idx;
DROP INDEX report_jobs_queue_idx;
ALTER TABLE report_jobs
  DROP COLUMN content, DROP COLUMN content_type, DROP COLUMN size_bytes, DROP COLUMN row_count,
  DROP COLUMN truncated, DROP COLUMN error, DROP COLUMN started_at, DROP COLUMN finished_at;
ALTER TABLE reconciliation_results DROP COLUMN confidence_breakdown;
DROP INDEX change_requests_one_open_per_finding;
DROP INDEX change_requests_status_idx;
ALTER TABLE change_requests
  DROP CONSTRAINT change_requests_no_self_decision,
  DROP COLUMN finding_id, DROP COLUMN reason, DROP COLUMN decision_note, DROP COLUMN applied_at;
DROP INDEX discovery_observations_sync_idx;
DROP INDEX integration_errors_job_idx;
DROP INDEX integration_sync_history_idx;
DROP INDEX integration_sync_queue_idx;
DROP INDEX integration_sync_one_active;
ALTER TABLE integration_sync_jobs
  DROP COLUMN created_at, DROP COLUMN requested_by, DROP COLUMN contexts, DROP COLUMN error;
ALTER TABLE integration_connections
  DROP COLUMN is_enabled, DROP COLUMN sync_interval_minutes, DROP COLUMN last_sync_at, DROP COLUMN last_error;
