-- A batch waiting for approval can be discarded. A file may come back after it was
-- discarded or reversed, so only live batches hold its hash.
ALTER TABLE import_batches DROP CONSTRAINT import_batches_status_check;
ALTER TABLE import_batches ADD CONSTRAINT import_batches_status_check CHECK (status IN
  ('uploaded','validated','approved','committed','reversed','failed','discarded'));
ALTER TABLE import_batches DROP CONSTRAINT import_batches_file_hash_key;
CREATE UNIQUE INDEX import_batches_live_file_idx ON import_batches (file_hash) WHERE status NOT IN ('reversed','discarded');
