GRANT DELETE ON devices TO dfipam_app;

-- The roles keep links:commercial_view: removing it could wipe costs again.

DROP TRIGGER segments_updated ON segments;
CREATE TRIGGER segments_updated BEFORE UPDATE ON segments FOR EACH ROW
  WHEN (OLD.scan_failed_at IS NOT DISTINCT FROM NEW.scan_failed_at)
  EXECUTE FUNCTION set_updated_at();
ALTER TABLE segments DROP COLUMN last_swept_at;
