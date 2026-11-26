package notifications

import (
	"context"
	"time"

	"dfipam/api/internal/platform/db"
)

// queued is one notice waiting for email, with the address it goes to: the
// person's, or the team mailbox for a notice with no user.
type queued struct {
	ID       string    `db:"id"`
	Key      string    `db:"recipient"` // user id, or "team"
	To       string    `db:"address"`
	Kind     string    `db:"kind"`
	Message  string    `db:"message"`
	Link     string    `db:"link"`
	Severity string    `db:"severity"`
	At       time.Time `db:"created_at"`
}

const queuedSQL = `SELECT n.id::text AS id, COALESCE(n.user_id::text, 'team') AS recipient,
	COALESCE(u.email, $1) AS address, n.kind, n.message, n.link, n.severity, n.created_at
	FROM notifications n LEFT JOIN users u ON u.id = n.user_id
	WHERE n.email_status = 'pending' AND (n.email_next_at IS NULL OR n.email_next_at <= now())`

// sendNow sends each notice due at once in its own email: a Critical alert,
// or a decision someone is waiting for.
func (o *Outbox) sendNow(ctx context.Context, cfg config) error {
	items, err := db.All[queued](ctx, o.conn, queuedSQL+` AND n.email_mode = 'now' ORDER BY n.created_at LIMIT $2`, cfg.Team, batchSize)
	if err != nil {
		return err
	}
	for _, n := range items {
		sendErr := o.deliver(ctx, single(n), n.To)
		if err := o.mark(ctx, []string{n.ID}, sendErr); err != nil {
			return err
		}
	}
	return nil
}

// sendDigests sends one email per person (or the team mailbox) whose oldest
// waiting notice is older than the digest interval, with everything waiting.
func (o *Outbox) sendDigests(ctx context.Context, cfg config) error {
	items, err := db.All[queued](ctx, o.conn, `WITH due AS (
		SELECT user_id FROM notifications WHERE email_status = 'pending' AND email_mode = 'digest'
		GROUP BY user_id HAVING min(created_at) <= now() - make_interval(mins => $2)
	) `+queuedSQL+` AND n.email_mode = 'digest'
		AND EXISTS (SELECT 1 FROM due WHERE due.user_id IS NOT DISTINCT FROM n.user_id)
		ORDER BY recipient, n.created_at LIMIT 500`, cfg.Team, cfg.DigestMinutes)
	if err != nil {
		return err
	}
	for _, group := range byRecipient(items) {
		ids := make([]string, len(group))
		for i, n := range group {
			ids[i] = n.ID
		}
		sendErr := o.deliver(ctx, digest(group, cfg.DigestMinutes), group[0].To)
		if err := o.mark(ctx, ids, sendErr); err != nil {
			return err
		}
	}
	return nil
}

// byRecipient splits notices, already sorted by recipient, into one group each.
func byRecipient(items []queued) [][]queued {
	var out [][]queued
	for i, n := range items {
		if i == 0 || n.Key != items[i-1].Key {
			out = append(out, nil)
		}
		out[len(out)-1] = append(out[len(out)-1], n)
	}
	return out
}
