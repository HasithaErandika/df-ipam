package notifications

import (
	"context"
	"log/slog"
	"time"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/mail"
)

const (
	// maxPerHour caps email to one person. Past it, notices stay in the app only,
	// so a burst of alerts can't flood anyone's mailbox.
	maxPerHour  = 20
	maxAttempts = 5
	batchSize   = 20
	// Read notifications older than this are removed; unread ones are kept.
	keepRead = "90 days"
)

type outboxItem struct {
	ID, Email, Kind, Message, Link string
	Attempts                       int
}

// RunOutbox delivers queued email in the API process. Email is sent only when
// Settings > Email is on and SMTP is configured; otherwise queued email is
// dropped while the in-app notification stays. A failed send is retried.
func RunOutbox(ctx context.Context, conn db.Conn, mailer mail.Mailer, baseURL string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	tick := 0
	for {
		if err := deliverBatch(ctx, conn, mailer, baseURL); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "notification outbox failed", "error", err)
		}
		if tick%720 == 0 { // about hourly
			if _, err := conn.Exec(ctx, `DELETE FROM notifications WHERE is_read AND created_at < now() - $1::interval`, keepRead); err != nil && ctx.Err() == nil {
				slog.ErrorContext(ctx, "notification cleanup failed", "error", err)
			}
		}
		tick++
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// EmailEnabled reads the switch in Settings. It is off until an Admin turns it on.
func EmailEnabled(ctx context.Context, conn db.Conn) bool {
	var on bool
	_ = conn.QueryRow(ctx, `SELECT COALESCE((SELECT (value #>> '{}')::boolean FROM settings WHERE key = 'email.enabled'), false)`).Scan(&on)
	return on
}

func deliverBatch(ctx context.Context, conn db.Conn, mailer mail.Mailer, baseURL string) error {
	if mailer == nil || !EmailEnabled(ctx, conn) {
		_, err := conn.Exec(ctx, `UPDATE notifications SET email_status = 'none' WHERE email_status = 'pending'`)
		return err
	}
	// Over the hourly cap, an email is skipped rather than delayed: the notice is
	// still in the app, and a late burst of stale mail helps nobody.
	if _, err := conn.Exec(ctx, `UPDATE notifications n SET email_status = 'none' WHERE n.email_status = 'pending'
		AND (SELECT count(*) FROM notifications s WHERE s.user_id = n.user_id AND s.email_status = 'sent' AND s.created_at > now() - interval '1 hour') >= $1`,
		maxPerHour); err != nil {
		return err
	}
	rows, err := conn.Query(ctx, `SELECT n.id::text, u.email, n.kind, n.message, n.link, n.email_attempts FROM notifications n
		JOIN users u ON u.id = n.user_id WHERE n.email_status = 'pending' ORDER BY n.created_at LIMIT $1`, batchSize)
	if err != nil {
		return err
	}
	var items []outboxItem
	for rows.Next() {
		var n outboxItem
		if err := rows.Scan(&n.ID, &n.Email, &n.Kind, &n.Message, &n.Link, &n.Attempts); err != nil {
			rows.Close()
			return err
		}
		items = append(items, n)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, n := range items {
		body := n.Message + "\n\nOpen DF-IPAM: " + baseURL + n.Link + "\n\nYou can turn these emails off under Alerts > Email notifications."
		sendErr := mailer.Send(ctx, n.Email, "DF-IPAM: "+subjects[n.Kind], body)
		status := "sent"
		if sendErr != nil {
			status = "pending"
			if n.Attempts+1 >= maxAttempts {
				status = "failed"
			}
			slog.WarnContext(ctx, "notification email failed", "notificationId", n.ID, "attempt", n.Attempts+1)
		}
		if _, err := conn.Exec(ctx, `UPDATE notifications SET email_status = $2, email_attempts = email_attempts + 1 WHERE id = $1::uuid AND email_status = 'pending'`,
			n.ID, status); err != nil {
			return err
		}
	}
	return nil
}

var subjects = map[string]string{
	"alert": "network alert", "server_up": "server back up", "request": "new IP request",
	"request_decision": "your IP request", "reservation_expiry": "reservation expiring",
	"link_contract_expiry": "network link contract expiring",
}
