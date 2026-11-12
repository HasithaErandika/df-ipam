-- Sign-in sessions for Microsoft Entra ID. The browser holds a random token in
-- a cookie scripts can't read; only its SHA-256 hash is stored here. A session
-- ends at expires_at, after the idle timeout, when revoked at sign-out, or as
-- soon as its user is deactivated.
CREATE TABLE sessions (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      uuid NOT NULL REFERENCES users (id),
  token_hash   bytea NOT NULL UNIQUE CHECK (length(token_hash) = 32),
  created_at   timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  expires_at   timestamptz NOT NULL,
  revoked_at   timestamptz,
  ip           inet,
  user_agent   text NOT NULL DEFAULT '' CHECK (length(user_agent) <= 300)
);
CREATE INDEX sessions_user_idx ON sessions (user_id);
CREATE INDEX sessions_expires_idx ON sessions (expires_at);

ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
CREATE POLICY dfipam_app_all ON sessions TO dfipam_app USING (true) WITH CHECK (true);
GRANT SELECT, INSERT, UPDATE, DELETE ON sessions TO dfipam_app;
DO $$
DECLARE r text;
BEGIN
  FOREACH r IN ARRAY ARRAY['anon', 'authenticated'] LOOP
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r) THEN
      EXECUTE format('REVOKE ALL ON sessions FROM %I', r);
    END IF;
  END LOOP;
END $$;
