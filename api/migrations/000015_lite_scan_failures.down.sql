DROP TRIGGER segments_updated ON segments;
CREATE TRIGGER segments_updated BEFORE UPDATE ON segments FOR EACH ROW EXECUTE FUNCTION set_updated_at();
ALTER TABLE segments DROP COLUMN scan_failed_at;
