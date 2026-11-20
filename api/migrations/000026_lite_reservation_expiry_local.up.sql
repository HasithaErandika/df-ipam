-- Reservation expiries were stored as midnight UTC after the chosen day. The API
-- now works in Sri Lanka time, so move them to midnight in Sri Lanka: the same
-- calendar day, ending when people there expect.
UPDATE reservations
SET expires_at = (expires_at AT TIME ZONE 'UTC') AT TIME ZONE 'Asia/Colombo'
WHERE expires_at IS NOT NULL
  AND (expires_at AT TIME ZONE 'UTC')::time = '00:00';
