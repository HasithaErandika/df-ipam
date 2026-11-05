DROP INDEX ip_addresses_state_idx;
DROP INDEX reconciliation_fingerprint_idx;
ALTER TABLE reconciliation_results DROP COLUMN fingerprint, DROP COLUMN last_detected_at, DROP COLUMN review_note;
