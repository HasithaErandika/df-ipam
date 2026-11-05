DROP INDEX notifications_unread_idx, notifications_outbox_idx, notifications_dedupe;
ALTER TABLE notifications DROP COLUMN next_attempt_at, DROP COLUMN last_error, DROP COLUMN object_id,
  DROP COLUMN object_type;

DROP TRIGGER ip_reservations_not_allocated ON ip_reservations;
DROP FUNCTION ip_reservations_block_allocated();
DROP TRIGGER ip_allocations_not_reserved ON ip_allocations;
DROP FUNCTION ip_allocations_block_reserved();

DROP INDEX ip_reservations_subnet_idx, ip_reservation_one_per_request;
DROP INDEX ip_allocations_subnet_idx, ip_alloc_one_per_request;
DROP INDEX ip_request_approvals_pending_idx, ip_request_approvals_one_per_person;
ALTER TABLE ip_request_approvals DROP COLUMN approver_role_id;

-- Rows in the new states can't satisfy the old check, so they are mapped back.
UPDATE ip_requests SET status = 'allocated' WHERE status IN ('reserved', 'released');
ALTER TABLE ip_requests DROP CONSTRAINT ip_requests_status_check;
ALTER TABLE ip_requests ADD CONSTRAINT ip_requests_status_check CHECK (status IN
  ('draft','submitted','pending_approval','approved','rejected','allocated','cancelled','expired'));
DROP INDEX ip_requests_allocation_idx, ip_requests_subnet_idx;
ALTER TABLE ip_requests DROP CONSTRAINT ip_requests_conversion_target, DROP CONSTRAINT ip_requests_release_target,
  DROP COLUMN fulfilled_by, DROP COLUMN fulfilled_at, DROP COLUMN decided_at, DROP COLUMN submitted_at,
  DROP COLUMN workflow_id, DROP COLUMN reservation_id, DROP COLUMN allocation_id, DROP COLUMN version;

ALTER TABLE ip_reservations DROP COLUMN reminded_at, DROP COLUMN released_at, DROP COLUMN released_by,
  DROP COLUMN request_id;
