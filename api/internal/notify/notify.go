// Package notify queues in-app notifications and the email that goes with
// them. Each is written in the caller's transaction, so a change and its
// notice commit together. Most notices stay in the app: email is kept for what
// someone must act on (docs/product/alerts.md#email).
package notify

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Delivery says whether, and how soon, a notice also goes by email.
type Delivery string

const (
	AppOnly Delivery = ""       // in the app only
	Now     Delivery = "now"    // its own email straight away: Critical alerts, a decision someone waits for
	Digest  Delivery = "digest" // gathered into one email every few minutes (Settings: email.digest_minutes)
)

// Kinds a person can switch email on or off for. The daily summary is built
// from what is open, not queued.
var Kinds = []string{"alert", "request", "request_decision", "daily_summary"}

// teamKinds go to the team mailbox instead of each person, when one is set.
var teamKinds = []string{"alert", "request"}

// User notifies one person. Email follows their preference for the kind.
func User(ctx context.Context, tx pgx.Tx, userID, kind, message, link string, d Delivery) error {
	_, err := tx.Exec(ctx, `INSERT INTO notifications (user_id, kind, message, link, email_mode, email_status)
		SELECT u.id, $2, $3, $4, $5, CASE WHEN $5 <> '' AND COALESCE(p.email, true) THEN 'pending' ELSE 'none' END
		FROM users u LEFT JOIN notification_preferences p ON p.user_id = u.id AND p.kind = $2
		WHERE u.id = $1::uuid AND u.is_active`, userID, kind, message, link, string(d))
	return err
}

// Holders notifies every active person whose role grants permission. When a
// team mailbox is set and the kind is a team matter, the email goes there
// once instead of to each person: a row with no user, which no inbox shows.
// severity colours the line in the email.
func Holders(ctx context.Context, tx pgx.Tx, permission, kind, message, link, severity string, d Delivery) error {
	_, err := tx.Exec(ctx, `WITH team AS (
		SELECT $5 <> '' AND $1 = ANY($7::text[]) AND `+TeamAddressSQL+` <> '' AS active
	), people AS (
		INSERT INTO notifications (user_id, kind, message, link, severity, email_mode, email_status)
		SELECT u.id, $1, $2, $3, $6, $5,
			CASE WHEN $5 <> '' AND NOT (SELECT active FROM team) AND COALESCE(p.email, true) THEN 'pending' ELSE 'none' END
		FROM users u LEFT JOIN notification_preferences p ON p.user_id = u.id AND p.kind = $1
		WHERE u.is_active AND EXISTS (SELECT 1 FROM roles r WHERE r.key = u.role AND (r.full_access OR $4 = ANY(r.permissions)))
		RETURNING 1
	) INSERT INTO notifications (user_id, kind, message, link, severity, email_mode, email_status, is_read)
		SELECT NULL, $1, $2, $3, $6, $5, 'pending', true FROM team WHERE team.active AND EXISTS (SELECT 1 FROM people)`,
		kind, message, link, permission, string(d), severity, teamKinds)
	return err
}

// TeamAddressSQL reads the team mailbox setting: empty when none is set.
const TeamAddressSQL = `COALESCE((SELECT value #>> '{}' FROM settings WHERE key = 'email.team_address'), '')`
