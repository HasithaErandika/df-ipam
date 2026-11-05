package notifications

import (
	"context"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/mail"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	maxAttempts = 5
	// Email still waiting after this long is no longer useful, so it is dropped.
	staleAfter = 7 * 24 * time.Hour
)

// Backoff is the wait before the next attempt after n failures.
func Backoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	d := time.Minute << (2 * (attempts - 1)) // 1m, 4m, 16m, 64m
	if d > 4*time.Hour {
		d = 4 * time.Hour
	}
	return d
}

// EmailText turns a payload into a subject and body. baseURL makes links absolute.
func EmailText(p Payload, baseURL string) (string, string) {
	body := p.Body
	if p.Link != "" && baseURL != "" {
		body += "\n\nOpen in DF-IPAM: " + strings.TrimRight(baseURL, "/") + p.Link
	}
	body += "\n\nYou can change which emails you receive under Alerts and Notifications."
	return "[DF-IPAM] " + p.Title, body
}

// DeliveryResult counts one outbox pass.
type DeliveryResult struct{ Sent, Failed, Suppressed int }

// Deliver suppresses stale emails, then sends the pending ones.
func Deliver(ctx context.Context, conn db.Conn, mailer mail.Mailer, baseURL string, batch int) (DeliveryResult, error) {
	var res DeliveryResult
	tag, err := conn.Exec(ctx, `UPDATE notifications SET status = 'suppressed', last_error = 'expired before delivery'
		WHERE channel = 'email' AND status = 'pending' AND created_at < now() - make_interval(secs => $1)`, staleAfter.Seconds())
	if err != nil {
		return res, err
	}
	res.Suppressed = int(tag.RowsAffected())
	if mailer == nil {
		return res, nil
	}
	err = db.InTx(ctx, conn, func(tx pgx.Tx) error {
		items, err := pendingEmails(ctx, tx, batch)
		if err != nil {
			return err
		}
		for _, it := range items {
			if err := sendEmail(ctx, tx, mailer, baseURL, it, &res); err != nil {
				return err
			}
		}
		return nil
	})
	return res, err
}

type pendingEmail struct {
	id, to   string
	payload  []byte
	attempts int
}

func pendingEmails(ctx context.Context, tx pgx.Tx, batch int) ([]pendingEmail, error) {
	rows, err := tx.Query(ctx, `SELECT n.id::text, u.email, n.payload, n.attempts FROM notifications n
		JOIN users u ON u.id = n.recipient_id
		WHERE n.channel = 'email' AND n.status = 'pending' AND (n.next_attempt_at IS NULL OR n.next_attempt_at <= now())
		ORDER BY n.created_at LIMIT $1 FOR UPDATE OF n SKIP LOCKED`, batch)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (pendingEmail, error) {
		var it pendingEmail
		return it, r.Scan(&it.id, &it.to, &it.payload, &it.attempts)
	})
}

func sendEmail(ctx context.Context, tx pgx.Tx, mailer mail.Mailer, baseURL string, it pendingEmail, res *DeliveryResult) error {
	var p Payload
	_ = json.Unmarshal(it.payload, &p)
	subject, body := EmailText(p, baseURL)
	attempts := it.attempts + 1
	if sendErr := mailer.Send(ctx, it.to, subject, body); sendErr != nil {
		res.Failed++
		return recordSendFailure(ctx, tx, it.id, attempts, sendErr)
	}
	res.Sent++
	_, err := tx.Exec(ctx, `UPDATE notifications SET attempts = $2, status = 'sent', sent_at = now(), last_error = NULL
		WHERE id = $1::uuid`, it.id, attempts)
	return err
}

func recordSendFailure(ctx context.Context, tx pgx.Tx, id string, attempts int, sendErr error) error {
	status := "pending"
	if attempts >= maxAttempts {
		status = "failed"
	}
	msg := sendErr.Error()
	if len(msg) > 300 {
		msg = msg[:300]
	}
	_, err := tx.Exec(ctx, `UPDATE notifications SET attempts = $2, status = $3, last_error = $4,
		next_attempt_at = now() + make_interval(secs => $5) WHERE id = $1::uuid`, id, attempts, status, msg, Backoff(attempts).Seconds())
	return err
}
