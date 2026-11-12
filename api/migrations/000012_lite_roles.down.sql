UPDATE users SET role = NULL WHERE role NOT IN ('admin', 'requester', 'viewer');
ALTER TABLE users DROP CONSTRAINT users_role_fkey;
DROP INDEX users_role_idx;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'requester', 'viewer'));
DROP TABLE roles;
