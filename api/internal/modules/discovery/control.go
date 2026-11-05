package discovery

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

func (s *Service) Control(ctx context.Context, actor, id, action string) error {
	if _, ok := transitions[action]; !ok {
		return httpx.ErrNotFound
	}
	return db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		return controlJob(ctx, tx, actor, id, action)
	})
}

func controlJob(ctx context.Context, tx pgx.Tx, actor, id, action string) error {
	t := transitions[action]
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM discovery_jobs WHERE id = $1::uuid FOR UPDATE`, id).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return httpx.ErrNotFound
	} else if err != nil {
		return err
	}
	if !slices.Contains(t.from, status) {
		return httpx.NewError(http.StatusConflict, "INVALID_STATE", "A "+status+" job can't be "+action+"d.")
	}
	if action == "resume" {
		if on, err := Stopped(ctx, tx); err != nil {
			return err
		} else if on {
			return errStopped
		}
	}
	finish := ""
	if t.to == "cancelled" {
		finish = ", finished_at = now()"
	}
	if _, err := tx.Exec(ctx, `UPDATE discovery_jobs SET status = $2`+finish+` WHERE id = $1::uuid`, id, t.to); err != nil {
		return err
	}
	if err := Log(ctx, tx, id, "info", "Job "+t.to+" by a person."); err != nil {
		return err
	}
	return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "discovery_job." + action, ObjectType: "discovery_job", ObjectID: id,
		Old: map[string]string{"status": status}, New: map[string]string{"status": t.to}})
}

// EmergencyStop turns the stop on or off. Turning it on stops every queued,
// running and paused job at once; agents halt at their next contact.
func (s *Service) EmergencyStop(ctx context.Context, actor string, on bool, reason string) (int, error) {
	if on && strings.TrimSpace(reason) == "" {
		return 0, httpx.Validation("reason", "Say why discovery is being stopped.")
	}
	n := 0
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) (err error) {
		if err := saveEmergencyStop(ctx, tx, actor, on); err != nil {
			return err
		}
		if on {
			if n, err = stopAllJobs(ctx, tx, reason); err != nil {
				return err
			}
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "discovery.emergency_stop", ObjectType: "system_setting",
			ObjectID: "discovery.emergency_stop", New: map[string]any{"active": on, "reason": reason, "jobsStopped": n}})
	})
	return n, err
}

func saveEmergencyStop(ctx context.Context, tx pgx.Tx, actor string, on bool) error {
	value := "false"
	if on {
		value = "true"
	}
	_, err := tx.Exec(ctx, `INSERT INTO system_settings (key, value, updated_by, updated_at) VALUES ('discovery.emergency_stop', $1::jsonb, $2::uuid, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()`, value, actor)
	return err
}

func stopAllJobs(ctx context.Context, tx pgx.Tx, reason string) (int, error) {
	rows, err := tx.Query(ctx, `UPDATE discovery_jobs SET status = 'stopped', finished_at = now(), error = 'Emergency stop: ' || $1
		WHERE status IN ('queued','running','paused') RETURNING id::text`, reason)
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, err
	}
	return len(ids), logEachJob(ctx, tx, ids, "error", "Stopped by the emergency stop: "+reason)
}

// Transitions people may make to a job.
var transitions = map[string]struct {
	from []string
	to   string
}{
	"pause":  {[]string{"queued", "running"}, "paused"},
	"resume": {[]string{"paused"}, "queued"},
	"cancel": {[]string{"queued", "running", "paused"}, "cancelled"},
}
