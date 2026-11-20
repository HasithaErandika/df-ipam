-- The Devices list counts each device's open alerts by MAC on every row; without
-- this index each count read every open alert.
CREATE INDEX alerts_mac_open_idx ON alerts (mac) WHERE status <> 'closed' AND mac IS NOT NULL;
