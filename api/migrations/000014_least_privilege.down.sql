UPDATE system_settings SET value = '500' WHERE key = 'discovery.max_total_pps' AND value = '100';
GRANT DELETE ON users TO dfipam_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON sessions TO dfipam_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON schema_migrations TO dfipam_app;
