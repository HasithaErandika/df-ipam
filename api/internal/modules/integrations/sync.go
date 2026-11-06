package integrations

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/platform/safehttp"
	"errors"

	"github.com/jackc/pgx/v5"
)

// TestResult reports a connection test.
type TestResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// Test makes one read against the platform.
func (s *Service) Test(ctx context.Context, actor, id string) (TestResult, error) {
	c, err := s.Get(ctx, id)
	if err != nil {
		return TestResult{}, err
	}
	conn, err := s.connector(ctx, c)
	var e *httpx.Error
	if errors.As(err, &e) {
		return TestResult{}, err
	}
	if err == nil {
		tctx, cancel := context.WithTimeout(ctx, testTimeout)
		defer cancel()
		err = conn.Test(tctx)
	}
	out := TestResult{OK: err == nil, Message: "Connected. The credentials can read this platform."}
	if err != nil {
		out.Message = safeMessage(err)
	}
	werr := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "integration.test", ObjectType: "integration_connection", ObjectID: id, New: out})
	})
	return out, werr
}

// safeMessage turns a connector failure into text for people and the database.
// Connectors put no secrets in errors, and this keeps policy refusals readable.
func safeMessage(err error) string {
	if errors.Is(err, safehttp.ErrBlocked) {
		return "The connection was refused by the outbound policy. Check the allowlist."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "The platform didn't answer in time."
	}
	return truncate(err.Error(), 500)
}

// RequestSync queues a sync started by a person.
func (s *Service) RequestSync(ctx context.Context, actor, id string) (SyncJob, error) {
	var job SyncJob
	err := db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		c, err := s.get(ctx, tx, id)
		if err != nil {
			return err
		}
		if !c.HasCredentials && c.Kind != KindAWS {
			return errNoCredential
		}
		var jobID string
		err = tx.QueryRow(ctx, `INSERT INTO integration_sync_jobs (connection_id, requested_by) VALUES ($1::uuid, $2::uuid)
			ON CONFLICT (connection_id) WHERE status IN ('queued', 'running') DO NOTHING RETURNING id::text`, id, actor).Scan(&jobID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errSyncActive
		}
		if err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "integration.sync_requested", ObjectType: "integration_connection", ObjectID: id,
			New: map[string]string{"jobId": jobID}}); err != nil {
			return err
		}
		job, err = s.syncJob(ctx, tx, jobID)
		return err
	})
	return job, err
}

const syncColumns = `j.id::text, j.connection_id::text, j.status, j.counts, j.contexts, j.error,
	(SELECT count(*) FROM integration_errors e WHERE e.sync_job_id = j.id), u.email, j.created_at, j.started_at, j.finished_at`

func scanSync(row pgx.Row) (SyncJob, error) {
	var j SyncJob
	return j, row.Scan(&j.ID, &j.ConnectionID, &j.Status, &j.Counts, &j.Contexts, &j.Error, &j.ErrorCount, &j.RequestedBy, &j.CreatedAt, &j.StartedAt, &j.FinishedAt)
}

func (s *Service) syncJob(ctx context.Context, q db.Conn, id string) (SyncJob, error) {
	return scanSync(q.QueryRow(ctx, `SELECT `+syncColumns+` FROM integration_sync_jobs j LEFT JOIN users u ON u.id = j.requested_by WHERE j.id = $1::uuid`, id))
}

// Syncs returns one page of a connection's history.
func (s *Service) Syncs(ctx context.Context, id string, p httpx.Page) ([]SyncJob, int64, error) {
	var total int64
	if err := s.conn.QueryRow(ctx, `SELECT count(*) FROM integration_sync_jobs WHERE connection_id = $1::uuid`, id).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.conn.Query(ctx, `SELECT `+syncColumns+` FROM integration_sync_jobs j LEFT JOIN users u ON u.id = j.requested_by
		WHERE j.connection_id = $1::uuid ORDER BY j.created_at DESC, j.id LIMIT $2 OFFSET $3`, id, p.Size, p.Offset())
	if err != nil {
		return nil, 0, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (SyncJob, error) { return scanSync(r) })
	return out, total, err
}

// Errors returns the problems a sync recorded.
func (s *Service) Errors(ctx context.Context, jobID string, p httpx.Page) ([]SyncError, int64, error) {
	var total int64
	if err := s.conn.QueryRow(ctx, `SELECT count(*) FROM integration_errors WHERE sync_job_id = $1::uuid`, jobID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.conn.Query(ctx, `SELECT code, message, context, occurred_at FROM integration_errors WHERE sync_job_id = $1::uuid
		ORDER BY occurred_at, id LIMIT $2 OFFSET $3`, jobID, p.Size, p.Offset())
	if err != nil {
		return nil, 0, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (SyncError, error) {
		var e SyncError
		return e, r.Scan(&e.Code, &e.Message, &e.Context, &e.OccurredAt)
	})
	return out, total, err
}
