// Package notify queues in-app notifications, which double as the email outbox.
// Each is written in the caller's transaction, so a change and its notice commit
// together. Whether an email goes out is decided later by the outbox: the global
// switch in Settings, the person's preference and the hourly cap.
package notify

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Kinds a person can switch email on or off for.
var Kinds = []string{"alert", "server_up", "request", "request_decision", "reservation_expiry"}

// User notifies one person. email asks for an email too, if their preference allows.
func User(ctx context.Context, tx pgx.Tx, userID, kind, message, link string, email bool) error {
	_, err := tx.Exec(ctx, `INSERT INTO notifications (user_id, kind, message, link, email_status)
		SELECT u.id, $2, $3, $4, CASE WHEN $5 AND COALESCE(p.email, true) THEN 'pending' ELSE 'none' END
		FROM users u LEFT JOIN notification_preferences p ON p.user_id = u.id AND p.kind = $2
		WHERE u.id = $1::uuid AND u.is_active`, userID, kind, message, link, email)
	return err
}

// Holders notifies every active person whose role grants permission. email
// asks for an email too, where their preference allows.
func Holders(ctx context.Context, tx pgx.Tx, permission, kind, message, link string, email bool) error {
	_, err := tx.Exec(ctx, `INSERT INTO notifications (user_id, kind, message, link, email_status)
		SELECT u.id, $1, $2, $3, CASE WHEN $5 AND COALESCE(p.email, true) THEN 'pending' ELSE 'none' END
		FROM users u LEFT JOIN notification_preferences p ON p.user_id = u.id AND p.kind = $1
		WHERE u.is_active AND EXISTS (SELECT 1 FROM roles r WHERE r.key = u.role AND (r.full_access OR $4 = ANY(r.permissions)))`,
		kind, message, link, permission, email)
	return err
}
