package discovery

import (
	"context"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type claimedTarget struct {
	ID              string  `json:"id"`
	RoutingDomainID string  `json:"routingDomainId"`
	CIDR            string  `json:"cidr"`
	Checkpoint      *string `json:"checkpoint"`
	Status          string  `json:"status"`
}

// defaultMaxTotalPPS caps all discovery running at once, in packets per second. Each
// policy has its own limit, but several agents together could otherwise add up to a
// flood on the network this system is meant to protect. hardMaxTotalPPS holds even if
// the setting is raised.
const (
	defaultMaxTotalPPS = 100
	hardMaxTotalPPS    = 500
)

// fitsBudget says whether a job can start. A job alone on the network always may,
// because its policy was approved at that rate, and it would otherwise never run.
func fitsBudget(running, jobPPS, ceiling int) bool {
	return running == 0 || running+jobPPS <= ceiling
}

// trafficBudget reads the ceiling, and what other agents use now. Agents that have
// gone silent don't count, so a crashed agent can't block discovery for good.
func trafficBudget(ctx context.Context, q querier, agent string) (ceiling, running int, err error) {
	err = q.QueryRow(ctx, `SELECT
		  COALESCE((SELECT (value #>> '{}')::int FROM system_settings WHERE key = 'discovery.max_total_pps'), $2),
		  COALESCE((SELECT sum((j.config_snapshot->>'maxPps')::int) FROM discovery_jobs j JOIN discovery_agents a ON a.id = j.agent_id
		            WHERE j.status = 'running' AND a.id <> $1::uuid AND a.last_heartbeat_at > now() - interval '5 minutes'), 0)`,
		agent, defaultMaxTotalPPS).Scan(&ceiling, &running)
	if ceiling < 1 {
		ceiling = defaultMaxTotalPPS
	}
	return min(ceiling, hardMaxTotalPPS), running, err
}

// grantedRate is what a job may actually use: its policy's rate, within what is left
// of the ceiling. Only this reaches the agent, so no job can exceed the ceiling.
func grantedRate(running, jobPPS, ceiling int) int {
	return max(1, min(jobPPS, ceiling-running))
}

// claimedJob is what an agent receives when it is given work.
type claimedJob struct {
	JobID   string          `json:"jobId"`
	Config  Snapshot        `json:"config"`
	Targets []claimedTarget `json:"targets"`
}

// claimCandidate is a job the agent could take, with what decides whether it may.
type claimCandidate struct {
	id       string
	snap     Snapshot
	resuming bool
	busy     bool // another running job holds an overlapping target
}

// claim hands the agent its next job, if discovery may run now. Jobs pinned to
// another agent, or outside their policy's windows, are left alone.
func (h *Handler) claim(c *gin.Context) {
	ctx, agent := c.Request.Context(), c.GetString(agentKey)
	var out *claimedJob
	err := db.InTx(ctx, h.svc.conn, func(tx pgx.Tx) (err error) {
		out, err = claimNext(ctx, tx, agent)
		return err
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if out == nil {
		c.Status(http.StatusNoContent)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

// claimNext starts the first job that may run now, or returns nil when none may.
func claimNext(ctx context.Context, tx pgx.Tx, agent string) (*claimedJob, error) {
	if on, err := Stopped(ctx, tx); err != nil || on {
		return nil, err
	}
	// One claim at a time, so two agents can't both pass the checks below.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('discovery-claim', 7))`); err != nil {
		return nil, err
	}
	list, err := claimCandidates(ctx, tx, agent)
	if err != nil {
		return nil, err
	}
	ceiling, running, err := trafficBudget(ctx, tx, agent)
	if err != nil {
		return nil, err
	}
	now := localNow()
	for _, cd := range list {
		// Waiting jobs stay queued and go at a later claim. Nothing is lost.
		if InWindow(cd.snap.Windows, now) && (cd.resuming || (!cd.busy && fitsBudget(running, cd.snap.MaxPPS, ceiling))) {
			cd.snap.MaxPPS = grantedRate(running, cd.snap.MaxPPS, ceiling)
			return startClaimed(ctx, tx, agent, cd)
		}
	}
	return nil, nil
}

// claimCandidates reads the agent's next jobs and their overlap checks in one query.
// A job this agent already runs comes first, so a restart resumes it.
func claimCandidates(ctx context.Context, tx pgx.Tx, agent string) ([]claimCandidate, error) {
	rows, err := tx.Query(ctx, `SELECT j.id::text, j.config_snapshot, j.status = 'running', EXISTS (
		  SELECT 1 FROM discovery_job_targets a
		  JOIN discovery_job_targets b ON b.routing_domain_id = a.routing_domain_id AND b.target && a.target AND b.job_id <> a.job_id
		  JOIN discovery_jobs other ON other.id = b.job_id AND other.status = 'running'
		  WHERE a.job_id = j.id)
		FROM discovery_jobs j
		WHERE (j.status = 'running' AND j.agent_id = $1::uuid) OR (j.status = 'queued' AND (j.agent_id IS NULL OR j.agent_id = $1::uuid))
		ORDER BY (j.status = 'running') DESC, j.priority, j.created_at LIMIT 20 FOR UPDATE OF j SKIP LOCKED`, agent)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (claimCandidate, error) {
		var cd claimCandidate
		var raw []byte
		if err := r.Scan(&cd.id, &raw, &cd.resuming, &cd.busy); err != nil {
			return cd, err
		}
		_ = json.Unmarshal(raw, &cd.snap)
		return cd, nil
	})
}

func startClaimed(ctx context.Context, tx pgx.Tx, agent string, cd claimCandidate) (*claimedJob, error) {
	if _, err := tx.Exec(ctx, `UPDATE discovery_jobs SET status = 'running', agent_id = $2::uuid, started_at = coalesce(started_at, now()), claimed_at = now()
		WHERE id = $1::uuid`, cd.id, agent); err != nil {
		return nil, err
	}
	// Exclusions and blackouts added since the job was frozen win before a packet goes out.
	if _, err := closeBlockedTargets(ctx, tx, cd.id); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT id::text, routing_domain_id::text, target::text, checkpoint, status FROM discovery_job_targets
		WHERE job_id = $1::uuid AND status IN ('pending','running') ORDER BY target`, cd.id)
	if err != nil {
		return nil, err
	}
	targets, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (claimedTarget, error) {
		var t claimedTarget
		return t, r.Scan(&t.ID, &t.RoutingDomainID, &t.CIDR, &t.Checkpoint, &t.Status)
	})
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, finishEmptyJob(ctx, tx, cd.id)
	}
	if err := Log(ctx, tx, cd.id, "info", "Claimed by agent."); err != nil {
		return nil, err
	}
	return &claimedJob{JobID: cd.id, Config: cd.snap, Targets: targets}, nil
}
