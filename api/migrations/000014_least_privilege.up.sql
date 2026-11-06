-- Least privilege for the application role. A SQL injection or stolen app password
-- must not be able to rewrite migration history or erase people. Accounts are
-- deactivated, never deleted, and the sessions table is unused: sign-in is token based.
-- No new tables, so row level security from earlier migrations still covers everything.
REVOKE ALL ON schema_migrations FROM dfipam_app;
REVOKE ALL ON sessions FROM dfipam_app;
REVOKE DELETE, TRUNCATE ON users FROM dfipam_app;

-- A safer ceiling on all discovery traffic. A value someone chose is left alone.
UPDATE system_settings SET value = '100' WHERE key = 'discovery.max_total_pps' AND value = '500';
