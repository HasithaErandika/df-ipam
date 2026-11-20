package links

import (
	"context"
	"log/slog"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
)

// RunReminders checks contract expiry every few hours and notifies each link's
// responsible officer and everyone who manages link contracts.
func RunReminders(ctx context.Context, conn db.Conn) {
	ticker := time.NewTicker(3 * time.Hour)
	defer ticker.Stop()
	for {
		if err := QueueReminders(ctx, conn); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "network link contract reminder check failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// QueueReminders sends one notice per link when its contract reaches a
// reminder day (90, 60, 30, 7 and 0 by default, in Sri Lanka time). Days
// already passed are marked as sent together, so a link added 20 days before
// expiry gets one notice, not three, and nothing is sent twice. A new contract
// end clears the marks (see auditSave).
func QueueReminders(ctx context.Context, conn db.Conn) error {
	_, err := conn.Exec(ctx, `WITH cfg AS (
		SELECT COALESCE((SELECT array_agg(d::integer) FROM settings, jsonb_array_elements_text(value) d WHERE key = 'links.reminder_days'), '{}') AS days
	), due AS (
		SELECT l.id, (l.contract_end - `+colomboToday+`) AS left_days,
			ARRAY(SELECT d FROM unnest(cfg.days) d WHERE l.contract_end - `+colomboToday+` <= d AND NOT d = ANY(l.reminders_sent)) AS hit
		FROM network_links l, cfg
		WHERE l.archived_at IS NULL AND l.status <> 'terminated' AND l.contract_end IS NOT NULL
		  AND l.contract_end >= `+colomboToday+` - 7
	), marked AS (
		UPDATE network_links l SET reminders_sent = l.reminders_sent || due.hit FROM due
		WHERE l.id = due.id AND cardinality(due.hit) > 0
		RETURNING l.id, l.code, l.name, l.officer_id, l.contract_end, due.left_days
	), recipients AS (
		SELECT m.*, m.officer_id AS user_id FROM marked m WHERE m.officer_id IS NOT NULL
		UNION
		SELECT m.*, u.id FROM marked m JOIN users u ON u.is_active AND `+auth.HolderSQL("'"+auth.LinksCommercialManage+"'")+`
	)
	INSERT INTO notifications (user_id, kind, message, link, email_status)
	SELECT r.user_id, 'link_contract_expiry',
		'Contract for ' || r.code || ' ' || r.name || CASE
			WHEN r.left_days > 1 THEN ' ends in ' || r.left_days || ' days, on ' || to_char(r.contract_end, 'DD Mon YYYY') || '.'
			WHEN r.left_days = 1 THEN ' ends tomorrow, ' || to_char(r.contract_end, 'DD Mon YYYY') || '.'
			WHEN r.left_days = 0 THEN ' ends today.'
			ELSE ' ended on ' || to_char(r.contract_end, 'DD Mon YYYY') || '.' END,
		'/network-links/' || r.id, CASE WHEN COALESCE(p.email, true) THEN 'pending' ELSE 'none' END
	FROM recipients r JOIN users u ON u.id = r.user_id AND u.is_active
	LEFT JOIN notification_preferences p ON p.user_id = r.user_id AND p.kind = 'link_contract_expiry'`)
	return err
}
