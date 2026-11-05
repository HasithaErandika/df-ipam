-- Keep updated_at current on every table that has the column.
DO $$
DECLARE t text;
BEGIN
  FOR t IN SELECT table_name FROM information_schema.columns
           WHERE table_schema = 'public' AND column_name = 'updated_at' AND table_name <> 'users'
             AND table_name IN (SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE')
  LOOP
    EXECUTE format('CREATE TRIGGER %I BEFORE UPDATE ON %I FOR EACH ROW EXECUTE FUNCTION set_updated_at()', t || '_updated', t);
  END LOOP;
END $$;

-- A subnet with an exception sits outside subnets_no_overlap in both directions.
-- This closes that gap: a normal subnet may not overlap an exception subnet unless
-- it has its own exception. It only runs when overlap-relevant columns change, so
-- editing a subnet's name is not blocked by an overlap that was already approved.
CREATE FUNCTION subnets_check_exception_overlap() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.archived_at IS NOT NULL OR NEW.overlap_exception_id IS NOT NULL THEN
    RETURN NEW;
  END IF;
  PERFORM pg_advisory_xact_lock(hashtextextended(NEW.routing_domain_id::text, 0));
  IF EXISTS (
    SELECT 1 FROM subnets s
    WHERE s.routing_domain_id = NEW.routing_domain_id AND s.id <> NEW.id
      AND s.archived_at IS NULL AND s.overlap_exception_id IS NOT NULL
      AND s.cidr && NEW.cidr
  ) THEN
    RAISE EXCEPTION 'subnet % overlaps an exception subnet in this routing domain', NEW.cidr
      USING ERRCODE = 'exclusion_violation', CONSTRAINT = 'subnets_no_overlap';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER subnets_exception_overlap
  BEFORE INSERT OR UPDATE OF cidr, routing_domain_id, overlap_exception_id, archived_at ON subnets
  FOR EACH ROW EXECUTE FUNCTION subnets_check_exception_overlap();

-- An IP record must sit inside its parent subnet.
CREATE FUNCTION ip_addresses_check_in_subnet() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM subnets s WHERE s.id = NEW.subnet_id AND NEW.address <<= s.cidr) THEN
    RAISE EXCEPTION 'address % is outside its subnet', NEW.address USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER ip_addresses_in_subnet BEFORE INSERT OR UPDATE OF address, subnet_id ON ip_addresses
  FOR EACH ROW EXECUTE FUNCTION ip_addresses_check_in_subnet();

-- Nobody approves their own request.
CREATE FUNCTION approvals_block_self() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.approver_id IS NOT NULL AND NEW.approver_id = (SELECT requested_by FROM ip_requests WHERE id = NEW.request_id) THEN
    RAISE EXCEPTION 'a user cannot approve their own request' USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER approvals_no_self BEFORE INSERT OR UPDATE ON ip_request_approvals
  FOR EACH ROW EXECUTE FUNCTION approvals_block_self();

-- A job's approved scope and configuration can't change after creation.
CREATE FUNCTION discovery_jobs_freeze_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.config_snapshot IS DISTINCT FROM OLD.config_snapshot THEN
    RAISE EXCEPTION 'discovery job configuration is immutable' USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER discovery_jobs_frozen BEFORE UPDATE ON discovery_jobs
  FOR EACH ROW EXECUTE FUNCTION discovery_jobs_freeze_snapshot();

-- Audit rows are immutable. Retention pruning sets dfipam.audit_prune using a
-- privileged role; the app role has no UPDATE or DELETE grant in any case.
CREATE FUNCTION audit_logs_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' AND current_setting('dfipam.audit_prune', true) = 'on' AND current_user <> 'dfipam_app' THEN
    RETURN OLD;
  END IF;
  RAISE EXCEPTION 'audit_logs is append-only' USING ERRCODE = 'insufficient_privilege';
END $$;
CREATE TRIGGER audit_logs_protect BEFORE UPDATE OR DELETE ON audit_logs
  FOR EACH ROW EXECUTE FUNCTION audit_logs_immutable();
CREATE TRIGGER audit_logs_no_truncate BEFORE TRUNCATE ON audit_logs
  FOR EACH STATEMENT EXECUTE FUNCTION audit_logs_immutable();

-- Runtime role: least privilege.
GRANT USAGE ON SCHEMA public TO dfipam_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO dfipam_app;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO dfipam_app;
REVOKE UPDATE, DELETE, TRUNCATE ON audit_logs, discovery_observations, discovery_job_logs FROM dfipam_app;
REVOKE DELETE ON discovery_jobs, discovery_job_targets FROM dfipam_app;
REVOKE UPDATE ON discovery_job_targets FROM dfipam_app;
GRANT UPDATE (checkpoint, status) ON discovery_job_targets TO dfipam_app;
