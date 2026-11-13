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

// deviceAlerts are about a MAC, wherever it is: a device that DHCP moves to
// another address keeps its one alert instead of raising a new one.
var deviceAlerts = map[string]bool{"unknown_device": true, "rejected_device": true, "private_mac_device": true}

func (c candidate) key() string {
	if deviceAlerts[c.typeName] && c.mac != "" {
		return c.typeName + ":" + c.mac
	}
	return fmt.Sprintf("%s:%s:%s", c.typeName, c.ip, c.mac)
}

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
		fresh, err := upsert(ctx, tx, found)
		if err != nil {
			return err
		}
		if err := announce(ctx, tx, fresh); err != nil {
			return err
		}
		keys := make([]string, len(found))
		for i, x := range found {
			keys[i] = x.key()
		}
		_, err = tx.Exec(ctx, closeSQL, keys)
		return err
	})
}

// upsert opens or refreshes every alert in one statement and returns the new
// ones that should be announced, in the rules' order.
func upsert(ctx context.Context, tx pgx.Tx, found []candidate) ([]candidate, error) {
	cols := make([][]string, 9)
	for _, x := range found {
		for i, v := range []string{x.typeName, x.severity, x.segmentID, x.ipID, x.deviceID, x.ip, x.mac, x.message, x.key()} {
			cols[i] = append(cols[i], v)
		}
	}
	rows, err := tx.Query(ctx, upsertSQL, cols[0], cols[1], cols[2], cols[3], cols[4], cols[5], cols[6], cols[7], cols[8])
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	announce := map[string]bool{}
	for rows.Next() {
		var key string
		var inserted, flapping bool
		if err := rows.Scan(&key, &inserted, &flapping); err != nil {
			return nil, err
		}
		announce[key] = inserted && !flapping
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var fresh []candidate
	for _, x := range found {
		if announce[x.key()] {
			fresh = append(fresh, x)
		}
	}
	return fresh, nil
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
