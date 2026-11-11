-- Evidence that an address is a gateway: it is the scanner's default route, or
-- the resolver named it the gateway. Shown with a router mark; the segment's own
-- gateway setting stays a person's decision.
ALTER TABLE sightings ADD COLUMN is_gateway boolean NOT NULL DEFAULT false;

-- Synthetic resolver names (such as "_gateway") are not hostnames.
UPDATE sightings SET is_gateway = true WHERE hostname = '_gateway';
UPDATE sightings SET hostname = '' WHERE hostname LIKE '\_%';
