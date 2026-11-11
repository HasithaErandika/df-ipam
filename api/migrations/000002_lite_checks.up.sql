ALTER TABLE ip_addresses
  ADD COLUMN server_check_failures integer NOT NULL DEFAULT 0 CHECK (server_check_failures >= 0);
