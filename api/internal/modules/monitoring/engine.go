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

// digestAfter is how many new alerts one evaluation may announce one by one.
// Past it, people get a single summary, so a noisy scan can't flood anyone.
const digestAfter = 5

// alertsPermission is who hears about alerts: those who may act on them.
const alertsPermission = "alerts:manage"

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
			if inserted && !flapping {
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

// announce tells those who handle alerts about each new one, in the app; High
// and Critical ones also by email. Many at once become one summary, so a noisy
// scan can't flood anyone.
func announce(ctx context.Context, tx pgx.Tx, fresh []candidate) error {
	if len(fresh) > digestAfter {
		urgent, critical := 0, 0
		for _, x := range fresh {
			if x.notifies() {
				urgent++
			}
			if x.severity == "critical" {
				critical++
			}
		}
		msg := fmt.Sprintf("%d new alerts, %d of them High or Critical (%d Critical). Open Alerts to review them.", len(fresh), urgent, critical)
		return notify.Holders(ctx, tx, alertsPermission, "alert", msg, "/alerts", urgent > 0)
	}
	for _, x := range fresh {
		if err := notify.Holders(ctx, tx, alertsPermission, "alert", x.message, "/alerts?q="+x.ip, x.notifies()); err != nil {
			return err
		}
	}
	return nil
}
