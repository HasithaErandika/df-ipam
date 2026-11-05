package discovery

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// resultReply tells an agent what to do next, and which targets it must now leave alone.
type resultReply struct {
	Instruction string   `json:"instruction"`
	SkipTargets []string `json:"skipTargets"`
}

// localNow is the time maintenance windows are written in.
func localNow() time.Time {
	loc, err := time.LoadLocation("Asia/Colombo")
	if err != nil {
		loc = time.FixedZone("Asia/Colombo", 5*3600+1800)
	}
	return time.Now().In(loc)
}

// holdToLimits applies the limits that can change while a job runs. A job past its
// window goes back to the queue, and the agent picks it up in the next window. Scope
// checks on results alone would be too late: by then the packets have been sent.
func holdToLimits(ctx context.Context, tx pgx.Tx, jobID string, windows []Window, reply resultReply) (resultReply, error) {
	if !InWindow(windows, localNow()) {
		if _, err := tx.Exec(ctx, `UPDATE discovery_jobs SET status = 'queued' WHERE id = $1::uuid AND status = 'running'`, jobID); err != nil {
			return reply, err
		}
		reply.Instruction = "pause"
		return reply, Log(ctx, tx, jobID, "warning", "Its maintenance window closed, so it waits for the next one.")
	}
	skip, err := closeBlockedTargets(ctx, tx, jobID)
	reply.SkipTargets = append(reply.SkipTargets, skip...)
	return reply, err
}

// closeBlockedTargets skips unfinished targets that an active exclusion or blackout
// now covers, and returns their IDs. A target only partly covered is skipped whole.
func closeBlockedTargets(ctx context.Context, tx pgx.Tx, jobID string) ([]string, error) {
	rows, err := tx.Query(ctx, `UPDATE discovery_job_targets t SET status = 'skipped'
		WHERE t.job_id = $1::uuid AND t.status IN ('pending','running') AND (
		  EXISTS (SELECT 1 FROM discovery_exclusions e WHERE (e.expires_at IS NULL OR e.expires_at > now())
		          AND (e.routing_domain_id IS NULL OR e.routing_domain_id = t.routing_domain_id) AND e.target && t.target)
		  OR EXISTS (SELECT 1 FROM discovery_blackouts b WHERE now() >= b.starts_at AND now() < b.ends_at
		          AND (b.target IS NULL OR b.target && t.target)))
		RETURNING t.id::text, t.target::text`, jobID)
	if err != nil {
		return nil, err
	}
	type closed struct{ id, target string }
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (closed, error) {
		var c closed
		return c, r.Scan(&c.id, &c.target)
	})
	if err != nil || len(list) == 0 {
		return nil, err
	}
	ids, targets := make([]string, len(list)), make([]string, len(list))
	for i, c := range list {
		ids[i], targets[i] = c.id, c.target
	}
	return ids, Log(ctx, tx, jobID, "warning", fmt.Sprintf("Skipped because of a new exclusion or blackout: %s", strings.Join(targets, ", ")))
}

// finishEmptyJob ends a job with nothing left to probe.
func finishEmptyJob(ctx context.Context, tx pgx.Tx, jobID string) error {
	if _, err := tx.Exec(ctx, `UPDATE discovery_jobs SET status = 'completed', progress = 100, finished_at = now() WHERE id = $1::uuid`, jobID); err != nil {
		return err
	}
	return Log(ctx, tx, jobID, "info", "Nothing was left to probe.")
}
