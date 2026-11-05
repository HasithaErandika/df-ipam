package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/modules/notifications"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

func (s *Service) runOne(ctx context.Context, jobID, connID string) error {
	c, err := s.Get(ctx, connID)
	if err != nil {
		return s.finishFailed(ctx, jobID, connID, "", err)
	}
	conn, err := s.connector(ctx, c)
	if err != nil {
		return s.finishFailed(ctx, jobID, connID, c.Name, err)
	}
	sctx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()
	result, err := conn.Collect(sctx)
	if err != nil {
		return s.finishFailed(ctx, jobID, connID, c.Name, err)
	}
	now := time.Now()
	err = db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		return recordSync(ctx, tx, jobID, c, result, now)
	})
	if err != nil {
		return s.finishFailed(ctx, jobID, connID, c.Name, err)
	}
	return nil
}

// recordSync stores what a sync collected and marks the job complete.
func recordSync(ctx context.Context, tx pgx.Tx, jobID string, c Connection, result *Result, now time.Time) error {
	counts, err := save(ctx, tx, c.ID, c.Kind, jobID, c.RoutingDomains, result, now)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(counts)
	if err != nil {
		return err
	}
	if err := recordWarnings(ctx, tx, jobID, result.Warnings); err != nil {
		return err
	}
	health := "healthy"
	if len(result.Warnings) > 0 {
		health = "degraded"
	}
	if _, err := tx.Exec(ctx, `UPDATE integration_sync_jobs SET status = 'completed', counts = $2::jsonb, contexts = $3, finished_at = now() WHERE id = $1::uuid`,
		jobID, raw, result.Contexts()); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE integration_connections SET health = $2, last_success_at = now(), last_sync_at = now(), last_error = '' WHERE id = $1::uuid`, c.ID, health); err != nil {
		return err
	}
	return audit.Write(ctx, tx, audit.Entry{ActorType: "system", Action: "integration.sync", ObjectType: "integration_connection", ObjectID: c.ID, New: counts})
}

func recordWarnings(ctx context.Context, tx pgx.Tx, jobID string, warnings []Warning) error {
	if len(warnings) == 0 {
		return nil
	}
	codes, messages, details := make([]string, len(warnings)), make([]string, len(warnings)), make([]string, len(warnings))
	for i, w := range warnings {
		detail, _ := json.Marshal(map[string]string{"context": w.Context})
		codes[i], messages[i], details[i] = w.Code, w.Message, string(detail)
	}
	_, err := tx.Exec(ctx, `INSERT INTO integration_errors (sync_job_id, code, message, context)
		SELECT $1::uuid, code, message, context::jsonb FROM unnest($2::text[], $3::text[], $4::text[]) AS t(code, message, context)`,
		jobID, codes, messages, details)
	return err
}

// RunResult summarises one pass of the sync loop.
type RunResult struct{ Queued, Completed, Failed, Stuck int64 }

// RunSyncs fails stuck jobs, queues connections that are due, then works through
// the queue. SKIP LOCKED lets several workers share it.
func (s *Service) RunSyncs(ctx context.Context) (RunResult, error) {
	var out RunResult
	tag, err := s.conn.Exec(ctx, `UPDATE integration_sync_jobs SET status = 'failed', error = 'The worker stopped before this sync finished.', finished_at = now()
		WHERE status = 'running' AND started_at < now() - make_interval(mins => $1)`, int(stuckAfter.Minutes()))
	if err != nil {
		return out, err
	}
	out.Stuck = tag.RowsAffected()
	tag, err = s.conn.Exec(ctx, `INSERT INTO integration_sync_jobs (connection_id)
		SELECT c.id FROM integration_connections c
		WHERE c.is_enabled AND (c.credential_ciphertext IS NOT NULL OR c.kind = 'aws')
		  AND c.kind IN ('fortigate', 'vcenter', 'aws', 'azure')
		  AND (c.last_sync_at IS NULL OR c.last_sync_at < now() - make_interval(mins => c.sync_interval_minutes))
		ON CONFLICT (connection_id) WHERE status IN ('queued', 'running') DO NOTHING`)
	if err != nil {
		return out, err
	}
	out.Queued = tag.RowsAffected()
	for ctx.Err() == nil {
		var jobID, connID string
		err := s.conn.QueryRow(ctx, `UPDATE integration_sync_jobs SET status = 'running', started_at = now()
			WHERE id = (SELECT id FROM integration_sync_jobs WHERE status = 'queued' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1)
			RETURNING id::text, connection_id::text`).Scan(&jobID, &connID)
		if errors.Is(err, pgx.ErrNoRows) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		if err := s.runOne(ctx, jobID, connID); err != nil {
			out.Failed++
			slog.Error("integration sync failed", "connection", connID, "job", jobID, "error", safeMessage(err))
			continue
		}
		out.Completed++
	}
	return out, ctx.Err()
}

func (s *Service) finishFailed(ctx context.Context, jobID, connID, name string, cause error) error {
	message := safeMessage(cause)
	var e *httpx.Error
	if errors.As(cause, &e) {
		message = e.Message
	}
	// The sync's own context may have ended, but the failure must still be recorded.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE integration_sync_jobs SET status = 'failed', error = $2, finished_at = now() WHERE id = $1::uuid`, jobID, message); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO integration_errors (sync_job_id, code, message) VALUES ($1::uuid, 'SYNC_FAILED', $2)`, jobID, message); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE integration_connections SET health = 'failed', last_sync_at = now(), last_error = $2 WHERE id = $1::uuid`, connID, message); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{ActorType: "system", Action: "integration.sync_failed", ObjectType: "integration_connection", ObjectID: connID,
			New: map[string]string{"error": message}}); err != nil {
			return err
		}
		recipients, err := notifications.UsersWithPermission(ctx, tx, "integrations:manage", nil)
		if err != nil {
			return err
		}
		return notifications.Enqueue(ctx, tx, "", notifications.Event{Type: notifications.IntegrationFailed, ObjectType: "integration_connection", ObjectID: connID,
			// One alert per connection per day, however often it retries.
			DedupeKey: fmt.Sprintf("integration-failed:%s:%s", connID, time.Now().UTC().Format("2006-01-02")),
			Title:     "Integration sync failed: " + name, Body: message, Recipients: recipients})
	})
	if err != nil {
		return errors.Join(cause, err)
	}
	return cause
}
