// Package notifications records alerts for people. Each event becomes one in-app
// row and, unless the person opted out, one email row per recipient. Rows are
// written in the caller's transaction, so a rolled-back change never notifies
// anyone. The worker delivers email rows through a Mailer.
package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"dfipam/api/internal/auth"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Querier is satisfied by a pool, a connection and a transaction.
type Querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// Event types. Each has a rule in notification_rules that can switch it off or
// change its severity.
const (
	ApprovalRequested   = "approval_requested"
	RequestApproved     = "request_approved"
	RequestRejected     = "request_rejected"
	RequestFulfilled    = "request_fulfilled"
	RequestReady        = "request_ready"
	RequestExpired      = "request_expired"
	ReservationExpiring = "reservation_expiring"
	ReservationExpired  = "reservation_expired"
	ReservationReleased = "reservation_released"
)

// EventType describes an event for rules, preferences and the seed.
type EventType struct {
	Type, Label, Severity, Recipients string
}

// EventTypes lists every event this build can raise, in display order.
var EventTypes = []EventType{
	{ApprovalRequested, "Approval requested", "info", "Eligible approvers"},
	{RequestApproved, "Request approved", "info", "Requester"},
	{RequestRejected, "Request rejected", "warning", "Requester"},
	{RequestReady, "Approved request ready to fulfil", "info", "People who can allocate or reserve"},
	{RequestFulfilled, "Request fulfilled", "info", "Requester"},
	{RequestExpired, "Request expired", "info", "Requester"},
	{ReservationExpiring, "Reservation nearing expiry", "warning", "Reservation holder"},
	{ReservationExpired, "Reservation expired", "warning", "Reservation holder"},
	{ReservationReleased, "Reservation released by someone else", "info", "Reservation holder"},
}

func knownType(t string) (EventType, bool) {
	for _, e := range EventTypes {
		if e.Type == t {
			return e, true
		}
	}
	return EventType{}, false
}

// Channels a person can receive on.
var Channels = []string{"in_app", "email"}

// Event is one thing worth telling people about.
type Event struct {
	Type       string
	ObjectType string
	ObjectID   string
	// DedupeKey stops the same event reaching the same person twice. It should
	// name the event and the object, plus anything that makes a repeat new.
	DedupeKey  string
	Title      string
	Body       string
	Link       string // path inside the web app, such as /requests?id=...
	Recipients []string
}

// Payload is what the in-app list and the email show.
type Payload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Link  string `json:"link,omitempty"`
}

func unique(ids []string, skip string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		if id != "" && id != skip && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// Enqueue records the event for its recipients in the caller's transaction.
// Disabled rules and opted-out channels write nothing. actor is left out of the
// recipients, since people don't need telling about their own actions.
func Enqueue(ctx context.Context, q Querier, actor string, e Event) error {
	def, ok := knownType(e.Type)
	if !ok {
		return fmt.Errorf("unknown notification event %q", e.Type)
	}
	recipients := unique(e.Recipients, actor)
	if len(recipients) == 0 {
		return nil
	}
	severity, enabled := def.Severity, true
	err := q.QueryRow(ctx, `SELECT severity, is_enabled FROM notification_rules WHERE event_type = $1`, e.Type).Scan(&severity, &enabled)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if !enabled {
		return nil
	}
	payload, err := json.Marshal(Payload{Title: e.Title, Body: e.Body, Link: e.Link})
	if err != nil {
		return err
	}
	var dedupe *string
	if e.DedupeKey != "" {
		dedupe = &e.DedupeKey
	}
	// In-app rows count as delivered once stored. Email rows wait for the worker.
	_, err = q.Exec(ctx, `
		INSERT INTO notifications (event_type, severity, recipient_id, channel, dedupe_key, payload, status, sent_at, object_type, object_id)
		SELECT $1, $2, u.id, ch.channel, $3, $4::jsonb,
		       CASE ch.channel WHEN 'in_app' THEN 'sent' ELSE 'pending' END,
		       CASE ch.channel WHEN 'in_app' THEN now() END, $5, $6
		FROM users u
		CROSS JOIN unnest($7::text[]) AS ch(channel)
		WHERE u.id = ANY($8::uuid[]) AND u.is_active
		  AND NOT EXISTS (SELECT 1 FROM notification_preferences p
		                  WHERE p.user_id = u.id AND p.event_type = $1 AND p.channel = ch.channel AND NOT p.is_enabled)
		ON CONFLICT (recipient_id, channel, dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING`,
		e.Type, severity, dedupe, payload, nullable(e.ObjectType), nullable(e.ObjectID), Channels, recipients)
	return err
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// UsersWithPermission returns active users holding the permission. A role, when
// given, must also be held. Used to find approvers and fulfillers.
func UsersWithPermission(ctx context.Context, q Querier, permission string, roleID *string) ([]string, error) {
	resource, action, _ := strings.Cut(permission, ":")
	rows, err := q.Query(ctx, `
		SELECT DISTINCT u.id::text FROM users u
		JOIN user_roles ur ON ur.user_id = u.id
		JOIN role_permissions rp ON rp.role_id = ur.role_id
		JOIN permissions pm ON pm.id = rp.permission_id
		WHERE u.is_active AND pm.resource = $1 AND pm.action = $2
		  AND ($3::uuid IS NULL OR EXISTS (SELECT 1 FROM user_roles r2 WHERE r2.user_id = u.id AND r2.role_id = $3::uuid))`,
		resource, action, roleID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// Approvers is UsersWithPermission for the approve permission.
func Approvers(ctx context.Context, q Querier, roleID *string) ([]string, error) {
	return UsersWithPermission(ctx, q, auth.RequestsApprove, roleID)
}
