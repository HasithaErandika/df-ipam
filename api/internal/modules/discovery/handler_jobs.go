package discovery

import (
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// Job is a discovery job with its frozen scope.
type Job struct {
	ID           string          `json:"id"`
	PolicyName   *string         `json:"policyName"`
	AgentName    *string         `json:"agentName"`
	Status       string          `json:"status"`
	Priority     int16           `json:"priority"`
	Progress     int16           `json:"progress"`
	Error        *string         `json:"error"`
	Snapshot     json.RawMessage `json:"config"`
	InitiatedBy  *string         `json:"initiatedByEmail"`
	Scheduled    bool            `json:"scheduled"`
	SubnetName   *string         `json:"subnetName"`
	Targets      int64           `json:"targets"`
	TargetsDone  int64           `json:"targetsDone"`
	Observations int64           `json:"observations"`
	CreatedAt    time.Time       `json:"createdAt"`
	StartedAt    *time.Time      `json:"startedAt"`
	FinishedAt   *time.Time      `json:"finishedAt"`
}

const jobCols = `j.id::text, p.name, a.name, j.status, j.priority, j.progress, j.error, j.config_snapshot, u.email, j.schedule_id IS NOT NULL, s.name,
	(SELECT count(*) FROM discovery_job_targets t WHERE t.job_id = j.id),
	(SELECT count(*) FROM discovery_job_targets t WHERE t.job_id = j.id AND t.status IN ('done','failed','skipped')),
	(SELECT count(*) FROM discovery_observations o WHERE o.job_id = j.id),
	j.created_at, j.started_at, j.finished_at`

const jobFrom = ` FROM discovery_jobs j LEFT JOIN discovery_policies p ON p.id = j.policy_id LEFT JOIN discovery_agents a ON a.id = j.agent_id
	LEFT JOIN users u ON u.id = j.initiated_by LEFT JOIN subnets s ON s.id = j.subnet_id`

func scanJob(r pgx.Row) (Job, error) {
	var j Job
	err := r.Scan(&j.ID, &j.PolicyName, &j.AgentName, &j.Status, &j.Priority, &j.Progress, &j.Error, &j.Snapshot, &j.InitiatedBy, &j.Scheduled,
		&j.SubnetName, &j.Targets, &j.TargetsDone, &j.Observations, &j.CreatedAt, &j.StartedAt, &j.FinishedAt)
	return j, err
}

var jobStatuses = map[string]bool{"queued": true, "running": true, "paused": true, "completed": true, "failed": true, "cancelled": true, "stopped": true}

func (h *Handler) jobs(c *gin.Context) {
	p := httpx.ParsePage(c)
	status := c.Query("status")
	if !jobStatuses[status] {
		status = ""
	}
	ctx := c.Request.Context()
	var total int64
	if err := h.svc.conn.QueryRow(ctx, `SELECT count(*) FROM discovery_jobs WHERE $1 = '' OR status = $1`, status).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := h.svc.conn.Query(ctx, "SELECT "+jobCols+jobFrom+" WHERE $1 = '' OR j.status = $1 ORDER BY j.created_at DESC, j.id LIMIT $2 OFFSET $3", status, p.Size, p.Offset())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Job, error) { return scanJob(r) })
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}

func (h *Handler) job(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	j, err := scanJob(h.svc.conn.QueryRow(c.Request.Context(), "SELECT "+jobCols+jobFrom+" WHERE j.id = $1::uuid", id))
	if errors.Is(err, pgx.ErrNoRows) {
		err = httpx.ErrNotFound
	}
	respond(c, http.StatusOK, j, err)
}

func (h *Handler) logs(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	rows, err := h.svc.conn.Query(c.Request.Context(), `SELECT level, message, logged_at FROM discovery_job_logs WHERE job_id = $1::uuid ORDER BY logged_at, id LIMIT 500`, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	type line struct {
		Level    string    `json:"level"`
		Message  string    `json:"message"`
		LoggedAt time.Time `json:"loggedAt"`
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (line, error) {
		var l line
		return l, r.Scan(&l.Level, &l.Message, &l.LoggedAt)
	})
	respond(c, http.StatusOK, append([]line{}, out...), err)
}

func (h *Handler) createJob(c *gin.Context) {
	var req JobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	if req.AgentID != nil && (*req.AgentID == "" || !httpx.IsUUID(*req.AgentID)) {
		req.AgentID = nil
	}
	ctx := c.Request.Context()
	var out Job
	err := db.InTx(ctx, h.svc.conn, func(tx pgx.Tx) error {
		id, err := h.svc.CreateJob(ctx, tx, auth.Current(c).UserID, req, nil)
		if err != nil {
			return err
		}
		out, err = scanJob(tx.QueryRow(ctx, "SELECT "+jobCols+jobFrom+" WHERE j.id = $1::uuid", id))
		return err
	})
	if db.ErrorCode(err) == db.CodeForeignKey {
		err = httpx.Validation("agentId", "That agent doesn't exist.")
	}
	respond(c, http.StatusCreated, out, err)
}

func (h *Handler) control(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := httpx.PathID(c)
		if !ok {
			return
		}
		if err := h.svc.Control(c.Request.Context(), auth.Current(c).UserID, id, action); err != nil {
			httpx.Fail(c, err)
			return
		}
		h.job(c)
	}
}
