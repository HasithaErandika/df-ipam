-- What runs on a device or server, set by a person: the operating system
-- (platform) and, for servers, the main services. Discovery never writes these;
-- the web app only suggests a likely platform from DHCP and hostname evidence.
ALTER TABLE devices ADD COLUMN platform text NOT NULL DEFAULT '' CHECK (platform ~ '^[a-z0-9_]{0,30}$');
ALTER TABLE ip_addresses ADD COLUMN platform text NOT NULL DEFAULT '' CHECK (platform ~ '^[a-z0-9_]{0,30}$');
ALTER TABLE ip_addresses ADD COLUMN services text[] NOT NULL DEFAULT '{}' CHECK (cardinality(services) <= 10);
