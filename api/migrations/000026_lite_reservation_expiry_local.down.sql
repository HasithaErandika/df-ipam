UPDATE reservations
SET expires_at = (expires_at AT TIME ZONE 'Asia/Colombo') AT TIME ZONE 'UTC'
WHERE expires_at IS NOT NULL
  AND (expires_at AT TIME ZONE 'Asia/Colombo')::time = '00:00';
