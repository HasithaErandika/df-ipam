-- Remove the starting departments that nothing uses.
DELETE FROM departments d
WHERE lower(d.name) IN ('it', 'finance', 'marketing', 'cyber security', 'management', 'hr')
  AND NOT EXISTS (SELECT 1 FROM devices v WHERE lower(btrim(v.department)) = lower(d.name))
  AND NOT EXISTS (SELECT 1 FROM ip_addresses a WHERE lower(btrim(a.department)) = lower(d.name));
