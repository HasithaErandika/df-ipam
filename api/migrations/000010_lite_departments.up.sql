-- Departments people pick from on devices and register entries. Those keep the
-- name as text, so existing entries stay valid; renaming a department renames
-- it on them in the same transaction (admin module).
CREATE TABLE departments (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name        text NOT NULL CHECK (btrim(name) <> '' AND length(name) <= 120),
  description text NOT NULL DEFAULT '' CHECK (length(description) <= 300),
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX departments_name_key ON departments (lower(name));
CREATE TRIGGER departments_updated BEFORE UPDATE ON departments FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE departments ENABLE ROW LEVEL SECURITY;
CREATE POLICY dfipam_app_all ON departments TO dfipam_app USING (true) WITH CHECK (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON departments TO dfipam_app;
DO $$
DECLARE r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['anon', 'authenticated'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r) THEN
      EXECUTE format('REVOKE ALL ON departments FROM %I', r);
    END IF;
  END LOOP;
END $$;

-- Start from the names already in use, so nothing typed before is lost.
INSERT INTO departments (name)
SELECT DISTINCT ON (lower(btrim(department))) btrim(department)
FROM (SELECT department FROM devices UNION ALL SELECT department FROM ip_addresses) d
WHERE btrim(department) <> '' AND length(btrim(department)) <= 120
ORDER BY lower(btrim(department)), btrim(department)
ON CONFLICT DO NOTHING;
