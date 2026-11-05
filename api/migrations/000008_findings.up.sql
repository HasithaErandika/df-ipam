-- Lets repeat runs update one finding instead of creating duplicates.
ALTER TABLE reconciliation_results
  ADD COLUMN fingerprint text,
  ADD COLUMN last_detected_at timestamptz NOT NULL DEFAULT now(),
  ADD COLUMN review_note text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX reconciliation_fingerprint_idx ON reconciliation_results (fingerprint);
CREATE INDEX ip_addresses_state_idx ON ip_addresses (allocation_state, observation_state);
