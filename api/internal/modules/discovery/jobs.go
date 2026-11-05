package discovery

import (
	"context"
	"dfipam/api/internal/platform/db"
	"time"

	"github.com/jackc/pgx/v5"
)

// OfflineAfter is how long an agent may be silent before it shows as offline.
const OfflineAfter = 5 * time.Minute

// MinScheduleGap is the shortest time between runs of one schedule, however often its
// cron says. Without it, a schedule of every minute would sweep without a break.
const MinScheduleGap = 15 * time.Minute

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
		return s.fireSchedules(ctx, tx, minute, &res)
	})
	return res, err
}

type schedule struct {
	id, policy, cron, tz string
	subnet               string
	agent                *string
	priority             int
}

func (s *Service) fireSchedules(ctx context.Context, tx pgx.Tx, minute time.Time, res *SchedulerResult) error {
	due, err := dueSchedules(ctx, tx, minute)
	if err != nil {
		return err
	}
	firing, ids := firingNow(due, minute)
	if len(firing) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE discovery_schedules SET last_run_at = $2 WHERE id = ANY($1::uuid[])`, ids, minute); err != nil {
		return err
	}
	busy, err := busySchedules(ctx, tx, ids)
	if err != nil {
		return err
	}
	for _, sc := range firing {
		// A schedule whose previous job is still going doesn't pile up another.
		if busy[sc.id] {
			continue
		}
		if err := s.startScheduled(ctx, tx, sc, res); err != nil {
			return err
		}
	}
	return nil
}

func dueSchedules(ctx context.Context, tx pgx.Tx, minute time.Time) ([]schedule, error) {
	rows, err := tx.Query(ctx, `SELECT id::text, policy_id::text, cron, timezone, subnet_id::text, agent_id::text, priority FROM discovery_schedules
		WHERE is_enabled AND subnet_id IS NOT NULL AND (last_run_at IS NULL OR last_run_at <= $1) FOR UPDATE SKIP LOCKED`, minute.Add(-MinScheduleGap))
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (schedule, error) {
		var sc schedule
		return sc, r.Scan(&sc.id, &sc.policy, &sc.cron, &sc.tz, &sc.subnet, &sc.agent, &sc.priority)
	})
}

// firingNow keeps the schedules whose cron matches the minute in their timezone.
func firingNow(due []schedule, minute time.Time) ([]schedule, []string) {
	var firing []schedule
	var ids []string
	for _, sc := range due {
		spec, err := ParseCron(sc.cron)
		if err != nil {
			continue
		}
		loc, err := time.LoadLocation(sc.tz)
		if err != nil {
			loc = time.UTC
		}
		if spec.Matches(minute.In(loc)) {
			firing, ids = append(firing, sc), append(ids, sc.id)
		}
	}
	return firing, ids
}

func busySchedules(ctx context.Context, tx pgx.Tx, ids []string) (map[string]bool, error) {
	rows, err := tx.Query(ctx, `SELECT DISTINCT schedule_id::text FROM discovery_jobs WHERE schedule_id = ANY($1::uuid[]) AND status IN ('queued','running','paused')`, ids)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	busy := make(map[string]bool, len(list))
	for _, id := range list {
		busy[id] = true
	}
	return busy, nil
}

// startScheduled creates one job in a savepoint, so a schedule that can't run
// doesn't undo the others.
func (s *Service) startScheduled(ctx context.Context, tx pgx.Tx, sc schedule, res *SchedulerResult) error {
	sp, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	id := sc.id
	if _, err := s.CreateJob(ctx, sp, "", JobRequest{PolicyID: sc.policy, SubnetIDs: []string{sc.subnet}, AgentID: sc.agent, Priority: sc.priority}, &id); err != nil {
		_ = sp.Rollback(ctx)
		res.Failed++
		return nil
	}
	if err := sp.Commit(ctx); err != nil {
		return err
	}
	res.Started++
	return nil
}
