package discovery

import (
	"context"
	"time"

	"dfipam/api/internal/platform/db"

	"github.com/jackc/pgx/v5"
)

// OfflineAfter is how long an agent may be silent before it shows as offline.
const OfflineAfter = 5 * time.Minute

// SchedulerResult counts one scheduler pass.
type SchedulerResult struct {
	Started, Failed, AgentsOffline int
}

// RunScheduler starts the jobs whose schedules are due this minute and marks
// silent agents offline. A schedule fires at most once per minute, however
// many workers run, because its row is locked and its last run recorded.
func (s *Service) RunScheduler(ctx context.Context, now time.Time) (SchedulerResult, error) {
	var res SchedulerResult
	tag, err := s.conn.Exec(ctx, `UPDATE discovery_agents SET status = 'offline' WHERE status = 'active' AND last_heartbeat_at < now() - make_interval(secs => $1)`,
		OfflineAfter.Seconds())
	if err != nil {
		return res, err
	}
	res.AgentsOffline = int(tag.RowsAffected())
	if on, err := Stopped(ctx, s.conn); err != nil || on {
		return res, err
	}
	minute := now.Truncate(time.Minute)
	err = db.InTx(ctx, s.conn, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id::text, policy_id::text, cron, timezone, subnet_id::text, agent_id::text, priority FROM discovery_schedules
			WHERE is_enabled AND subnet_id IS NOT NULL AND (last_run_at IS NULL OR last_run_at < $1) FOR UPDATE SKIP LOCKED`, minute)
		if err != nil {
			return err
		}
		type schedule struct {
			id, policy, cron, tz string
			subnet               string
			agent                *string
			priority             int
		}
		var due []schedule
		for rows.Next() {
			var sc schedule
			if err := rows.Scan(&sc.id, &sc.policy, &sc.cron, &sc.tz, &sc.subnet, &sc.agent, &sc.priority); err != nil {
				rows.Close()
				return err
			}
			due = append(due, sc)
		}
		rows.Close()
		for _, sc := range due {
			spec, err := ParseCron(sc.cron)
			if err != nil {
				continue
			}
			loc, err := time.LoadLocation(sc.tz)
			if err != nil {
				loc = time.UTC
			}
			if !spec.Matches(minute.In(loc)) {
				continue
			}
			if _, err := tx.Exec(ctx, `UPDATE discovery_schedules SET last_run_at = $2 WHERE id = $1::uuid`, sc.id, minute); err != nil {
				return err
			}
			// A schedule whose previous job is still going doesn't pile up another.
			var busy bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM discovery_jobs WHERE schedule_id = $1::uuid AND status IN ('queued','running','paused'))`, sc.id).Scan(&busy); err != nil {
				return err
			}
			if busy {
				continue
			}
			id := sc.id
			sp, err := tx.Begin(ctx)
			if err != nil {
				return err
			}
			if _, err := s.CreateJob(ctx, sp, "", JobRequest{PolicyID: sc.policy, SubnetIDs: []string{sc.subnet}, AgentID: sc.agent, Priority: sc.priority}, &id); err != nil {
				_ = sp.Rollback(ctx)
				res.Failed++
				if _, lerr := tx.Exec(ctx, `UPDATE discovery_schedules SET last_run_at = $2 WHERE id = $1::uuid`, sc.id, minute); lerr != nil {
					return lerr
				}
				continue
			}
			if err := sp.Commit(ctx); err != nil {
				return err
			}
			res.Started++
		}
		return nil
	})
	return res, err
}
