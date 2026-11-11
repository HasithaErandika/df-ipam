package notifications

import (
	"context"
	"log/slog"
	"time"

	"dfipam/api/internal/platform/db"
)

// RunReservationReminders queues a one-time reminder in the week before expiry.
func RunReservationReminders(ctx context.Context, conn db.Conn) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		if err := queueReservationReminders(ctx, conn); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "reservation reminder check failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func queueReservationReminders(ctx context.Context, conn db.Conn) error {
	_, err := conn.Exec(ctx, `WITH due AS (
		UPDATE reservations SET reminder_sent_at=now()
		WHERE status='active' AND reminder_sent_at IS NULL AND expires_at>now() AND expires_at<=now()+make_interval(days => COALESCE((SELECT (value #>> '{}')::integer FROM settings WHERE key='reservations.reminder_days'),7))
		RETURNING owner_id,owner_name,start_ip,end_ip,reason
	), recipients AS (
		SELECT d.owner_id AS user_id,d.reason,d.start_ip,d.end_ip FROM due d WHERE d.owner_id IS NOT NULL
		UNION ALL SELECT u.id,d.reason,d.start_ip,d.end_ip FROM due d JOIN users u ON u.role='admin' AND u.is_active WHERE d.owner_id IS NULL
	)
	INSERT INTO notifications(user_id,kind,message,link,email_status)
	SELECT r.user_id,'reservation_expiry','Reservation expires soon: '||r.reason||' ('||host(r.start_ip)||CASE WHEN r.start_ip<>r.end_ip THEN ' – '||host(r.end_ip) ELSE '' END||')','/reservations',
		CASE WHEN COALESCE(p.email,true) THEN 'pending' ELSE 'none' END
	FROM recipients r LEFT JOIN notification_preferences p ON p.user_id=r.user_id AND p.kind='reservation_expiry'`)
	return err
}
