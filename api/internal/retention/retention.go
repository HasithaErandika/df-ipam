// Package retention removes data older than its retention policy. It runs as the
// owner role, never the app: discovery_observations and audit_logs are append-only
// for the app role, and that is the point of them.
package retention

import (
	"context"
	"errors"
	"fmt"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/platform/db"

	"github.com/jackc/pgx/v5"
)

const batch = 5000

// target is one prunable class. The statements are fixed: nothing comes from a request.
type target struct{ class, sql string }

var targets = []target{
	{"discovery_observations", `DELETE FROM discovery_observations WHERE id IN (
		SELECT o.id FROM discovery_observations o WHERE o.observed_at < now() - make_interval(days => $1)
		  AND NOT EXISTS (SELECT 1 FROM reconciliation_evidence e WHERE e.observation_id = o.id) LIMIT $2)`},
	{"discovery_job_logs", `DELETE FROM discovery_job_logs WHERE id IN (
		SELECT id FROM discovery_job_logs WHERE logged_at < now() - make_interval(days => $1) LIMIT $2)`},
	{"agent_heartbeats", `DELETE FROM agent_heartbeats WHERE id IN (
		SELECT id FROM agent_heartbeats WHERE received_at < now() - make_interval(days => $1) LIMIT $2)`},
	{"notifications", `DELETE FROM notifications WHERE id IN (
		SELECT id FROM notifications WHERE created_at < now() - make_interval(days => $1) AND status <> 'pending' LIMIT $2)`},
	{"report_jobs", `DELETE FROM report_jobs WHERE id IN (
		SELECT id FROM report_jobs WHERE created_at < now() - make_interval(days => $1) AND status IN ('completed', 'failed') LIMIT $2)`},
}

// Result counts what each class lost.
type Result map[string]int64

// Run prunes every class with a policy. Audit logs are left alone: their period
// needs compliance sign-off first, so they are removed only by an explicit decision.
func Run(ctx context.Context, conn db.Conn) (Result, error) {
	out := Result{}
	for _, t := range targets {
		var days int
		err := conn.QueryRow(ctx, `SELECT retain_days FROM retention_policies WHERE data_class = $1`, t.class).Scan(&days)
		if errors.Is(err, pgx.ErrNoRows) {
			continue // no policy means keep everything
		}
		if err != nil {
			return out, err
		}
		for {
			tag, err := conn.Exec(ctx, t.sql, days, batch)
			if err != nil {
				return out, fmt.Errorf("%s: %w", t.class, err)
			}
			out[t.class] += tag.RowsAffected()
			if tag.RowsAffected() < batch {
				break
			}
		}
	}
	return out, db.InTx(ctx, conn, func(tx pgx.Tx) error {
		return audit.Write(ctx, tx, audit.Entry{ActorType: "system", Action: "retention.prune", ObjectType: "retention_policy", New: out})
	})
}
