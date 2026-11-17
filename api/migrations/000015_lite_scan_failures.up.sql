-- When a segment's last scan failed, and since when. Its alerts then stay open:
-- evidence that wasn't collected is not evidence that the cause went away.
ALTER TABLE segments ADD COLUMN scan_failed_at timestamptz;

-- The scanner setting or clearing it is not an edit, so it leaves updated_at alone.
DROP TRIGGER segments_updated ON segments;
CREATE TRIGGER segments_updated BEFORE UPDATE ON segments FOR EACH ROW
  WHEN (OLD.scan_failed_at IS NOT DISTINCT FROM NEW.scan_failed_at)
  EXECUTE FUNCTION set_updated_at();
