-- Fixes from the November 2026 code review.

-- When each segment's sweep last finished. The scanner's schedule used the newest
-- ARP sighting, which the FortiGate reader also writes, so a restart could wait
-- a whole interval before the first sweep.
ALTER TABLE segments ADD COLUMN last_swept_at timestamptz;

-- The scanner recording a sweep is not an edit, so it leaves updated_at alone.
DROP TRIGGER segments_updated ON segments;
CREATE TRIGGER segments_updated BEFORE UPDATE ON segments FOR EACH ROW
  WHEN (OLD.scan_failed_at IS NOT DISTINCT FROM NEW.scan_failed_at
    AND OLD.last_swept_at IS NOT DISTINCT FROM NEW.last_swept_at)
  EXECUTE FUNCTION set_updated_at();

-- Managing link costs needs seeing them: a role with manage but not view was
-- sent the costs blank and wiped them on save. The API now adds view itself.
UPDATE roles SET permissions = array_append(permissions, 'links:commercial_view')
  WHERE 'links:commercial_manage' = ANY(permissions) AND NOT ('links:commercial_view' = ANY(permissions));

-- Devices are never deleted, so the ids in network_links.device_ids (an array,
-- which can't carry a foreign key) stay valid. Make that a rule, not a habit.
REVOKE DELETE ON devices FROM dfipam_app;
