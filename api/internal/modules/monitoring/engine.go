// Package monitoring evaluates sightings and server health into deduplicated alerts.
// It only reports: it never changes the register, devices' decisions or the network.
package monitoring

import (
	"context"
	"fmt"

	"dfipam/api/internal/notify"
	"dfipam/api/internal/platform/db"

	"github.com/jackc/pgx/v5"
)

// digestAfter is how many new notifying alerts one evaluation may announce one by
// one. Past it, Admins get a single summary, so a noisy scan can't flood inboxes.
const digestAfter = 5

// lockKey serialises evaluations: discovery and server checks both run them.
const lockKey = 7_301_001

type Engine struct{ conn db.Conn }

func New(conn db.Conn) *Engine { return &Engine{conn: conn} }

type candidate struct {
	typeName, severity, segmentID, ipID, deviceID, ip, mac, message string
}

func (c candidate) key() string { return fmt.Sprintf("%s:%s:%s", c.typeName, c.ip, c.mac) }

func (c candidate) notifies() bool { return c.severity == "high" || c.severity == "critical" }

func (e *Engine) Evaluate(ctx context.Context) error {
	return db.InTx(ctx, e.conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockKey); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, pendingDevicesSQL); err != nil {
			return err
		}
		found, err := candidates(ctx, tx)
		if err != nil {
			return err
		}
		keys := make([]string, 0, len(found))
		var fresh []candidate
		for _, x := range found {
			keys = append(keys, x.key())
			var inserted, flapping bool
			if err := tx.QueryRow(ctx, upsertSQL, x.typeName, x.severity, x.segmentID, x.ipID, x.deviceID, x.ip, x.mac,
				x.message, x.key()).Scan(&inserted, &flapping); err != nil {
				return err
			}
			if inserted && !flapping && x.notifies() {
				fresh = append(fresh, x)
			}
		}
		if err := announce(ctx, tx, fresh); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, closeSQL, keys)
		return err
	})
}

func candidates(ctx context.Context, tx pgx.Tx) ([]candidate, error) {
	rows, err := tx.Query(ctx, candidatesSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var found []candidate
	seen := map[string]bool{}
	for rows.Next() {
		var x candidate
		if err := rows.Scan(&x.typeName, &x.severity, &x.segmentID, &x.ipID, &x.deviceID, &x.ip, &x.mac, &x.message); err != nil {
			return nil, err
		}
		if !seen[x.key()] { // two rules can describe one cause; keep the first
			seen[x.key()] = true
			found = append(found, x)
		}
	}
	return found, rows.Err()
}

// announce tells Admins about new High and Critical alerts: one by one when there
// are few, or as one summary when a scan finds many at once.
func announce(ctx context.Context, tx pgx.Tx, fresh []candidate) error {
	if len(fresh) > digestAfter {
		critical := 0
		for _, x := range fresh {
			if x.severity == "critical" {
				critical++
			}
		}
		msg := fmt.Sprintf("%d new High or Critical alerts (%d Critical). Open Alerts to review them.", len(fresh), critical)
		return notify.Admins(ctx, tx, "alert", msg, "/alerts")
	}
	for _, x := range fresh {
		if err := notify.Admins(ctx, tx, "alert", x.message, "/alerts"); err != nil {
			return err
		}
	}
	return nil
}
