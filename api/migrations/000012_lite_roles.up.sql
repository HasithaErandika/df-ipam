-- Roles that Admins set up: a name and the permissions it grants. A role with
-- full_access holds every permission, including ones added later; it is a
-- system role and can't be changed or deleted. users.role names a role by key.
CREATE TABLE roles (
  key         text PRIMARY KEY CHECK (key ~ '^[a-z][a-z0-9_]{1,39}$'),
  name        text NOT NULL CHECK (btrim(name) <> '' AND length(name) <= 60),
  description text NOT NULL DEFAULT '' CHECK (length(description) <= 300),
  permissions text[] NOT NULL DEFAULT '{}' CHECK (cardinality(permissions) <= 64),
  full_access boolean NOT NULL DEFAULT false,
  is_system   boolean NOT NULL DEFAULT false,
  sort_order  integer NOT NULL DEFAULT 100,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX roles_name_key ON roles (lower(name));
CREATE TRIGGER roles_updated BEFORE UPDATE ON roles FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE roles ENABLE ROW LEVEL SECURITY;
CREATE POLICY dfipam_app_all ON roles TO dfipam_app USING (true) WITH CHECK (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON roles TO dfipam_app;
DO $$
DECLARE r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['anon', 'authenticated'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r) THEN
      EXECUTE format('REVOKE ALL ON roles FROM %I', r);
    END IF;
  END LOOP;
END $$;

-- Starting roles. Admins change or add to them in Access > Roles.
INSERT INTO roles (key, name, description, permissions, full_access, is_system, sort_order) VALUES
  ('admin', 'Administrator', 'Everything, including people, roles and system settings.', '{}', true, true, 10),
  ('network_engineer', 'Network engineer', 'Runs the network: segments, discovery, the register and devices, approves IP requests and handles alerts.',
   '{inventory:view,inventory:edit,network:manage,discovery:scan,ip:request,ip:reserve,requests:approve,alerts:manage,reports:export}', false, false, 20),
  ('it_officer', 'IT officer', 'Day-to-day: registers addresses and devices, raises requests and reservations, handles alerts.',
   '{inventory:view,inventory:edit,ip:request,ip:reserve,alerts:manage,reports:export}', false, false, 30),
  ('security_auditor', 'Security auditor', 'Reads everything, the audit log included, and exports reports. Changes nothing.',
   '{inventory:view,audit:view,reports:export}', false, false, 40),
  ('requester', 'Requester', 'Reads, and asks for IP addresses and reservations.',
   '{inventory:view,ip:request,ip:reserve,reports:export}', false, false, 50),
  ('viewer', 'Viewer', 'Reads and exports reports.', '{inventory:view,reports:export}', false, false, 60);

ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_fkey FOREIGN KEY (role) REFERENCES roles (key) ON UPDATE CASCADE ON DELETE RESTRICT;
CREATE INDEX users_role_idx ON users (role);
