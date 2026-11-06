-- Indexes for the queries that run most often: every agent call reads its job's
-- targets, each sync replaces a device's routes, and subnet pages and the daily
-- snapshot read reserved ranges and DHCP scopes. No new tables, so the row level
-- security and grants from earlier migrations still cover everything.

CREATE INDEX discovery_job_targets_job_idx ON discovery_job_targets (job_id);
CREATE INDEX discovery_job_targets_domain_idx ON discovery_job_targets (routing_domain_id);
CREATE INDEX discovery_jobs_agent_idx ON discovery_jobs (agent_id, status);
CREATE INDEX discovery_jobs_schedule_idx ON discovery_jobs (schedule_id) WHERE schedule_id IS NOT NULL;

CREATE INDEX network_routes_device_idx ON network_routes (device_id, source_id);
CREATE INDEX dhcp_scopes_subnet_idx ON dhcp_scopes (subnet_id);
CREATE INDEX subnet_reserved_ranges_subnet_idx ON subnet_reserved_ranges (subnet_id);
CREATE INDEX dns_records_ip_address_idx ON dns_records (ip_address_id);

-- Permission and approver lookups start from the role.
CREATE INDEX user_roles_role_idx ON user_roles (role_id);
CREATE INDEX role_permissions_permission_idx ON role_permissions (permission_id);
CREATE INDEX ip_requests_workflow_idx ON ip_requests (workflow_id);
