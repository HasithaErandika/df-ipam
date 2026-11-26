package notifications

import (
	"context"
	"log/slog"
	"time"

	"dfipam/api/internal/notify"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/mail"
)

const (
	maxAttempts = 5
	batchSize   = 50
	// Read notifications older than this are removed; unread ones are kept.
	keepRead = "90 days"
)

// Health lists problems with the background readers (FortiGate, Intune) for
// the daily summary. Nothing when they are fine.
type Health func() []notify.Row

// Outbox sends email in the API process, only when Settings > Email is on and
// SMTP is configured; otherwise notices stay in the app. It sends what is due
// now, each digest that is due, and the daily summary once a day.
type Outbox struct {
	conn   db.Conn
	mailer mail.Mailer
	base   string
	health Health
}

func NewOutbox(conn db.Conn, mailer mail.Mailer, baseURL string, health Health) *Outbox {
	return &Outbox{conn: conn, mailer: mailer, base: baseURL, health: health}
}

// Run checks every 30 seconds until ctx ends.
func (o *Outbox) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for tick := 0; ; tick++ {
		if err := o.Tick(ctx); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "email outbox failed", "error", err)
		}
		if tick%120 == 0 { // about hourly
			if _, err := o.conn.Exec(ctx, `DELETE FROM notifications WHERE is_read AND created_at < now() - $1::interval`, keepRead); err != nil && ctx.Err() == nil {
				slog.ErrorContext(ctx, "notification cleanup failed", "error", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Tick does one round: what is due now, due digests and the daily summary.
func (o *Outbox) Tick(ctx context.Context) error {
	cfg := loadConfig(ctx, o.conn)
	// With email off, or nowhere to send it, a notice stays in the app only.
	if _, err := o.conn.Exec(ctx, `UPDATE notifications n SET email_status = 'none' WHERE n.email_status = 'pending'
		AND (NOT $1 OR (n.user_id IS NULL AND $2 = '') OR EXISTS (SELECT 1 FROM users u WHERE u.id = n.user_id AND NOT u.is_active))`,
		o.mailer != nil && cfg.Enabled, cfg.Team); err != nil {
		return err
	}
	if o.mailer == nil || !cfg.Enabled {
		return nil
	}
	if err := o.sendNow(ctx, cfg); err != nil {
		return err
	}
	if err := o.sendDigests(ctx, cfg); err != nil {
		return err
	}
	return o.dailySummary(ctx, cfg)
}

// config is the email settings (Settings > Email).
type config struct {
	Enabled       bool
	DigestMinutes int
	SummaryHour   int
	Team          string
}

func loadConfig(ctx context.Context, conn db.Conn) config {
	c := config{DigestMinutes: 30, SummaryHour: 8}
	_ = conn.QueryRow(ctx, `SELECT
		COALESCE((SELECT (value #>> '{}')::boolean FROM settings WHERE key = 'email.enabled'), false),
		COALESCE((SELECT (value #>> '{}')::integer FROM settings WHERE key = 'email.digest_minutes'), 30),
		COALESCE((SELECT (value #>> '{}')::integer FROM settings WHERE key = 'email.summary_hour'), 8),
		`+notify.TeamAddressSQL).Scan(&c.Enabled, &c.DigestMinutes, &c.SummaryHour, &c.Team)
	return c
}

// EmailEnabled reads the switch in Settings. It is off until an Admin turns it on.
func EmailEnabled(ctx context.Context, conn db.Conn) bool { return loadConfig(ctx, conn).Enabled }

// mark records a send: sent, or another try 1, 2, 4 then 8 minutes later, and
// failed after the last.
func (o *Outbox) mark(ctx context.Context, ids []string, sendErr error) error {
	if sendErr == nil {
		_, err := o.conn.Exec(ctx, `UPDATE notifications SET email_status = 'sent', email_attempts = email_attempts + 1
			WHERE id = ANY($1::uuid[])`, ids)
		return err
	}
	o.logFailure(ctx, "notices", sendErr)
	_, err := o.conn.Exec(ctx, `UPDATE notifications SET email_attempts = email_attempts + 1,
		email_status = CASE WHEN email_attempts + 1 >= $2 THEN 'failed' ELSE 'pending' END,
		email_next_at = now() + interval '1 minute' * power(2, email_attempts) WHERE id = ANY($1::uuid[])`, ids, maxAttempts)
	return err
}

func (o *Outbox) logFailure(ctx context.Context, what string, err error) {
	slog.WarnContext(ctx, "email failed", "email", what, "error", err)
}

// deliver renders one email and sends it.
func (o *Outbox) deliver(ctx context.Context, e notify.Email, to string) error {
	letter, err := notify.Render(e, o.base, to)
	if err != nil {
		return err
	}
	return o.mailer.Send(ctx, letter)
}
