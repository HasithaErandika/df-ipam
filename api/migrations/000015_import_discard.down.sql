DROP INDEX import_batches_live_file_idx;
UPDATE import_batches SET status = 'failed' WHERE status = 'discarded';
ALTER TABLE import_batches DROP CONSTRAINT import_batches_status_check;
ALTER TABLE import_batches ADD CONSTRAINT import_batches_status_check CHECK (status IN
  ('uploaded','validated','approved','committed','reversed','failed'));
ALTER TABLE import_batches ADD CONSTRAINT import_batches_file_hash_key UNIQUE (file_hash);
