REVOKE ALL ON ALL TABLES IN SCHEMA public FROM dfipam_app;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM dfipam_app;
REVOKE USAGE ON SCHEMA public FROM dfipam_app;
DROP TRIGGER audit_logs_no_truncate ON audit_logs;
DROP TRIGGER audit_logs_protect ON audit_logs;
DROP FUNCTION audit_logs_immutable();
DROP TRIGGER discovery_jobs_frozen ON discovery_jobs;
DROP FUNCTION discovery_jobs_freeze_snapshot();
DROP TRIGGER approvals_no_self ON ip_request_approvals;
DROP FUNCTION approvals_block_self();
DROP TRIGGER ip_addresses_in_subnet ON ip_addresses;
DROP FUNCTION ip_addresses_check_in_subnet();
DROP TRIGGER subnets_exception_overlap ON subnets;
DROP FUNCTION subnets_check_exception_overlap();
DO $$
DECLARE r record;
BEGIN
  FOR r IN SELECT tgname, c.relname FROM pg_trigger t JOIN pg_class c ON c.oid = t.tgrelid
           WHERE NOT t.tgisinternal AND t.tgname LIKE '%\_updated' AND c.relname <> 'users'
  LOOP
    EXECUTE format('DROP TRIGGER %I ON %I', r.tgname, r.relname);
  END LOOP;
END $$;
