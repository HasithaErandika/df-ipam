package notifications

import (
	"context"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/notify"
	"dfipam/api/internal/platform/db"
)

// reader is one person who gets the daily summary, with what their role allows.
type reader struct {
	ID          string   `db:"id"`
	Email       string   `db:"email"`
	Role        string   `db:"role"`
	FullAccess  bool     `db:"full_access"`
	Permissions []string `db:"permissions"`
}

// dailySummary sends the summary once a day, after the hour set in Settings
// (Sri Lanka time). Claiming the day in settings first means a restart or a
// second process never sends it twice.
func (o *Outbox) dailySummary(ctx context.Context, cfg config) error {
	now := time.Now().In(sriLanka)
	if now.Hour() < cfg.SummaryHour {
		return nil
	}
	tag, err := o.conn.Exec(ctx, `INSERT INTO settings (key, value) VALUES ('email.summary_sent_on', to_jsonb($1::text))
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now() WHERE settings.value <> EXCLUDED.value`, now.Format(time.DateOnly))
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	f, top, err := readFacts(ctx, o.conn)
	if err != nil {
		return err
	}
	var health []notify.Row
	if o.health != nil {
		health = o.health()
	}
	readers, err := db.All[reader](ctx, o.conn, `SELECT u.id::text AS id, u.email, u.role, r.full_access, r.permissions
		FROM users u JOIN roles r ON r.key = u.role
		LEFT JOIN notification_preferences p ON p.user_id = u.id AND p.kind = 'daily_summary'
		WHERE u.is_active AND COALESCE(p.email, true) ORDER BY u.email`)
	if err != nil {
		return err
	}
	b := summary{f: f, top: top, health: health, day: now}
	if cfg.Team != "" { // the team mailbox gets every team section once; people then get only their own
		if e, ok := b.build(nil, true); ok {
			o.sendSummary(ctx, e, cfg.Team)
		}
	}
	for _, r := range readers {
		p := auth.NewPrincipal(r.ID, r.Email, r.Role, r.Permissions, r.FullAccess)
		if e, ok := b.build(p, cfg.Team == ""); ok {
			o.sendSummary(ctx, e, r.Email)
		}
	}
	return nil
}

// sendSummary sends one summary. A failure is logged and not retried: the next
// one comes tomorrow, and the app shows everything meanwhile.
func (o *Outbox) sendSummary(ctx context.Context, e notify.Email, to string) {
	if err := o.deliver(ctx, e, to); err != nil {
		o.logFailure(ctx, "daily summary", err)
	}
}
