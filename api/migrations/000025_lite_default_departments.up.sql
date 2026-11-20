-- The departments Dialog Finance starts with. A migration rather than the seed,
-- so one an Administrator deletes or renames later does not come back.
INSERT INTO departments (name) VALUES
  ('IT'), ('Finance'), ('Marketing'), ('Cyber Security'), ('Management'), ('HR')
ON CONFLICT ((lower(name))) DO NOTHING;
