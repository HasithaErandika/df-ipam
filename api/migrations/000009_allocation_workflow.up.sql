-- Phase 2: the allocation workflow. Requests are approved step by step, then
-- fulfilled as allocations, reservations or releases. No new tables, so the
-- row level security and grants from earlier migrations still cover everything.

-- Requests: an edit version, the record a release or conversion acts on, and
-- the workflow and timestamps the approval frozen at submission.
ALTER TABLE ip_requests
  ADD COLUMN version integer NOT NULL DEFAULT 1,
  ADD COLUMN allocation_id uuid REFERENCES ip_allocations(id),
  ADD COLUMN reservation_id uuid REFERENCES ip_reservations(id),
  ADD COLUMN workflow_id uuid REFERENCES approval_workflows(id),
  ADD COLUMN submitted_at timestamptz,
  ADD COLUMN decided_at timestamptz,
  ADD COLUMN fulfilled_at timestamptz,
  ADD COLUMN fulfilled_by uuid REFERENCES users(id),
  ADD CONSTRAINT ip_requests_release_target CHECK (request_type <> 'release' OR allocation_id IS NOT NULL),
  ADD CONSTRAINT ip_requests_conversion_target CHECK (reservation_id IS NULL OR request_type = 'allocation');

-- 'reserved' and 'released' are the fulfilled states of reservation and release
-- requests, as 'allocated' is for allocation requests.
ALTER TABLE ip_requests DROP CONSTRAINT ip_requests_status_check;
ALTER TABLE ip_requests ADD CONSTRAINT ip_requests_status_check CHECK (status IN
  ('draft','submitted','pending_approval','approved','rejected','allocated','reserved','released','cancelled','expired'));
CREATE INDEX ip_requests_subnet_idx ON ip_requests (subnet_id);
CREATE INDEX ip_requests_allocation_idx ON ip_requests (allocation_id) WHERE allocation_id IS NOT NULL;

-- Approval steps keep the role the workflow required when the request was
-- submitted, so later workflow edits don't change requests in flight. A null
-- role means anyone with the approve permission.
ALTER TABLE ip_request_approvals ADD COLUMN approver_role_id uuid REFERENCES roles(id);
-- One person decides at most one step of a request: multi-level approval needs
-- different people.
CREATE UNIQUE INDEX ip_request_approvals_one_per_person ON ip_request_approvals (request_id, approver_id)
  WHERE approver_id IS NOT NULL;
CREATE INDEX ip_request_approvals_pending_idx ON ip_request_approvals (request_id, step_order) WHERE decision = 'pending';

-- A request is fulfilled at most once, which also makes fulfilment idempotent.
CREATE UNIQUE INDEX ip_alloc_one_per_request ON ip_allocations (request_id) WHERE request_id IS NOT NULL;
CREATE INDEX ip_allocations_subnet_idx ON ip_allocations (subnet_id, status);

ALTER TABLE ip_reservations
  ADD COLUMN request_id uuid REFERENCES ip_requests(id),
  ADD COLUMN released_by uuid REFERENCES users(id),
  ADD COLUMN released_at timestamptz,
  ADD COLUMN reminded_at timestamptz;
CREATE UNIQUE INDEX ip_reservation_one_per_request ON ip_reservations (request_id) WHERE request_id IS NOT NULL;
CREATE INDEX ip_reservations_subnet_idx ON ip_reservations (subnet_id, status);

-- Allocations and reservations are separate tables, so their own unique indexes
-- can't stop one address being active in both. The service settles this through
-- a conditional update on the single ip_addresses row; these triggers back it up.
CREATE FUNCTION ip_allocations_block_reserved() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status = 'active' AND EXISTS (
    SELECT 1 FROM ip_reservations r
    WHERE r.routing_domain_id = NEW.routing_domain_id AND r.address = NEW.address AND r.status = 'active'
  ) THEN
    RAISE EXCEPTION 'address % has an active reservation', NEW.address
      USING ERRCODE = 'unique_violation', CONSTRAINT = 'ip_alloc_one_active';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER ip_allocations_not_reserved BEFORE INSERT OR UPDATE OF status ON ip_allocations
  FOR EACH ROW EXECUTE FUNCTION ip_allocations_block_reserved();

CREATE FUNCTION ip_reservations_block_allocated() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status = 'active' AND EXISTS (
    SELECT 1 FROM ip_allocations a
    WHERE a.routing_domain_id = NEW.routing_domain_id AND a.address = NEW.address AND a.status = 'active'
  ) THEN
    RAISE EXCEPTION 'address % has an active allocation', NEW.address
      USING ERRCODE = 'unique_violation', CONSTRAINT = 'ip_reservation_one_active';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER ip_reservations_not_allocated BEFORE INSERT OR UPDATE OF status ON ip_reservations
  FOR EACH ROW EXECUTE FUNCTION ip_reservations_block_allocated();

-- Notifications: one row per recipient, channel and event key, so a repeated
-- event never floods anyone. Email rows carry their delivery attempts.
ALTER TABLE notifications
  ADD COLUMN object_type text,
  ADD COLUMN object_id text,
  ADD COLUMN last_error text,
  ADD COLUMN next_attempt_at timestamptz;
CREATE UNIQUE INDEX notifications_dedupe ON notifications (recipient_id, channel, dedupe_key)
  WHERE dedupe_key IS NOT NULL;
CREATE INDEX notifications_outbox_idx ON notifications (next_attempt_at, created_at)
  WHERE status = 'pending' AND channel = 'email';
CREATE INDEX notifications_unread_idx ON notifications (recipient_id) WHERE channel = 'in_app' AND read_at IS NULL;
