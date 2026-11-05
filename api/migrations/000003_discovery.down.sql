ALTER TABLE subnets DROP CONSTRAINT subnets_discovery_policy_id_fkey, DROP CONSTRAINT subnets_discovery_schedule_id_fkey;
DROP TABLE reconciliation_evidence, reconciliation_results, discovery_observations, integration_errors,
  external_resource_mappings, integration_sync_jobs, integration_connections, agent_heartbeats,
  discovery_exclusions, discovery_job_logs, discovery_job_targets, discovery_jobs, discovery_schedules,
  discovery_policies, discovery_agents;
