package notifications

import (
	"context"
	"log/slog"
	"time"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/mail"
)

type outboxItem struct {
	ID, Email, Kind, Message string
	Attempts                 int
}

// RunOutbox delivers queued email in the API process. With SMTP disabled,
// queued mail is cleared while the in-app notification remains available.
func RunOutbox(ctx context.Context, conn db.Conn, mailer mail.Mailer) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if err := deliverBatch(ctx, conn, mailer); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "notification outbox failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

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
		WHERE status='active' AND reminder_sent_at IS NULL AND expires_at>now() AND expires_at<=now()+interval '7 days'
		RETURNING owner_id,owner_name,start_ip,end_ip,reason
	), recipients AS (
		SELECT d.owner_id AS user_id,d.reason,d.start_ip,d.end_ip FROM due d WHERE d.owner_id IS NOT NULL
		UNION ALL SELECT u.id,d.reason,d.start_ip,d.end_ip FROM due d JOIN users u ON u.role='admin' AND u.is_active WHERE d.owner_id IS NULL
	)
	INSERT INTO notifications(user_id,kind,message,link,email_status)
	SELECT r.user_id,'reservation_expiry','Reservation expires soon: '||r.reason||' ('||r.start_ip||'–'||r.end_ip||')','/reservations',
		CASE WHEN COALESCE(p.email,true) THEN 'pending' ELSE 'none' END
	FROM recipients r LEFT JOIN notification_preferences p ON p.user_id=r.user_id AND p.kind='reservation_expiry'`)
	return err
}

func deliverBatch(ctx context.Context, conn db.Conn, mailer mail.Mailer) error {
	rows, err := conn.Query(ctx, `SELECT n.id::text,u.email,n.kind,n.message,n.email_attempts FROM notifications n JOIN users u ON u.id=n.user_id WHERE n.email_status='pending' ORDER BY n.created_at LIMIT 20`)
	if err != nil {
		return err
	}
	var items []outboxItem
	for rows.Next() {
		var n outboxItem
		if err := rows.Scan(&n.ID, &n.Email, &n.Kind, &n.Message, &n.Attempts); err != nil {
			rows.Close()
			return err
		}
		items = append(items, n)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, n := range items {
		if mailer == nil {
			_, err = conn.Exec(ctx, `UPDATE notifications SET email_status='none' WHERE id=$1::uuid AND email_status='pending'`, n.ID)
		} else {
			err = mailer.Send(ctx, n.Email, "DF-IPAM notification", n.Message)
			status := "sent"
			if err != nil && n.Attempts+1 < 5 {
				status = "pending"
			} else if err != nil {
				status = "failed"
			}
			_, dbErr := conn.Exec(ctx, `UPDATE notifications SET email_status=$2,email_attempts=email_attempts+1 WHERE id=$1::uuid AND email_status='pending'`, n.ID, status)
			if dbErr != nil {
				return dbErr
			}
			if err != nil {
				slog.WarnContext(ctx, "notification email delivery failed", "notificationId", n.ID, "attempt", n.Attempts+1)
			}
			err = nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}
