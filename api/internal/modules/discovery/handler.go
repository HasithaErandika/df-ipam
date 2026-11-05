package discovery

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc} }

var viewDiscovery = []string{auth.DiscoveryRun, auth.DiscoveryManage, auth.AuditView}

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/discovery/status", h.status, viewDiscovery...)
	g.Handle(http.MethodPost, "/discovery/emergency-stop", h.emergencyStop, auth.DiscoveryRun, auth.DiscoveryManage)
	g.Handle(http.MethodGet, "/discovery/policies", h.policies, viewDiscovery...)
	g.Handle(http.MethodPost, "/discovery/policies", h.createPolicy, auth.DiscoveryManage)
	g.Handle(http.MethodPut, "/discovery/policies/:id", h.updatePolicy, auth.DiscoveryManage)
	g.Handle(http.MethodPost, "/discovery/policies/:id/approve", h.approvePolicy, auth.DiscoveryManage)
	g.Handle(http.MethodGet, "/discovery/jobs", h.jobs, viewDiscovery...)
	g.Handle(http.MethodPost, "/discovery/jobs", h.createJob, auth.DiscoveryRun)
	g.Handle(http.MethodGet, "/discovery/jobs/:id", h.job, viewDiscovery...)
	g.Handle(http.MethodGet, "/discovery/jobs/:id/logs", h.logs, viewDiscovery...)
	g.Handle(http.MethodPost, "/discovery/jobs/:id/pause", h.control("pause"), auth.DiscoveryRun)
	g.Handle(http.MethodPost, "/discovery/jobs/:id/resume", h.control("resume"), auth.DiscoveryRun)
	g.Handle(http.MethodPost, "/discovery/jobs/:id/cancel", h.control("cancel"), auth.DiscoveryRun)
	g.Handle(http.MethodGet, "/agents", h.agents, viewDiscovery...)
	g.Handle(http.MethodPost, "/agents", h.createAgent, auth.DiscoveryManage)
	g.Handle(http.MethodPost, "/agents/:id/rotate", h.rotateAgent, auth.DiscoveryManage)
	g.Handle(http.MethodPost, "/agents/:id/revoke", h.revokeAgent, auth.DiscoveryManage)
}

func respond(c *gin.Context, status int, v any, err error) {
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, status, v)
}

func (h *Handler) status(c *gin.Context) {
	ctx := c.Request.Context()
	var out struct {
		EmergencyStop bool       `json:"emergencyStop"`
		StoppedAt     *time.Time `json:"changedAt"`
		Queued        int64      `json:"queued"`
		Running       int64      `json:"running"`
		AgentsOnline  int64      `json:"agentsOnline"`
		AgentsTotal   int64      `json:"agentsTotal"`
	}
	err := h.svc.conn.QueryRow(ctx, `SELECT
		coalesce((SELECT value = 'true'::jsonb FROM system_settings WHERE key = 'discovery.emergency_stop'), false),
		(SELECT updated_at FROM system_settings WHERE key = 'discovery.emergency_stop'),
		(SELECT count(*) FROM discovery_jobs WHERE status = 'queued'), (SELECT count(*) FROM discovery_jobs WHERE status = 'running'),
		(SELECT count(*) FROM discovery_agents WHERE status = 'active'), (SELECT count(*) FROM discovery_agents WHERE status <> 'revoked')`).
		Scan(&out.EmergencyStop, &out.StoppedAt, &out.Queued, &out.Running, &out.AgentsOnline, &out.AgentsTotal)
	respond(c, http.StatusOK, out, err)
}

func (h *Handler) emergencyStop(c *gin.Context) {
	var req struct {
		Active bool   `json:"active"`
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	// Anyone allowed to run discovery may stop it; lifting the stop needs the manage permission.
	if !req.Active && !auth.Current(c).Has(auth.DiscoveryManage) {
		httpx.Fail(c, httpx.ErrForbidden)
		return
	}
	n, err := h.svc.EmergencyStop(c.Request.Context(), auth.Current(c).UserID, req.Active, req.Reason)
	respond(c, http.StatusOK, gin.H{"emergencyStop": req.Active, "jobsStopped": n}, err)
}

func (h *Handler) policies(c *gin.Context) {
	out, err := h.svc.Policies(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, httpx.Page{Number: 1, Size: len(out)}, int64(len(out)))
}

func (h *Handler) savePolicy(c *gin.Context, id string, status int) {
	var in PolicyInput
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	p, err := h.svc.SavePolicy(c.Request.Context(), auth.Current(c).UserID, id, in)
	respond(c, status, p, err)
}

func (h *Handler) createPolicy(c *gin.Context) { h.savePolicy(c, "", http.StatusCreated) }

func (h *Handler) updatePolicy(c *gin.Context) {
	if id, ok := httpx.PathID(c); ok {
		h.savePolicy(c, id, http.StatusOK)
	}
}

func (h *Handler) approvePolicy(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	p, err := h.svc.ApprovePolicy(c.Request.Context(), auth.Current(c).UserID, id)
	respond(c, http.StatusOK, p, err)
}

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

// Agent is a discovery agent. Its credential never leaves the server.
type Agent struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	SiteName        *string    `json:"siteName"`
	Status          string     `json:"status"`
	Version         string     `json:"version"`
	LastHeartbeatAt *time.Time `json:"lastHeartbeatAt"`
	RotatedAt       *time.Time `json:"credentialRotatedAt"`
	RevokedAt       *time.Time `json:"revokedAt"`
	PendingToken    bool       `json:"registrationPending"`
	CreatedAt       time.Time  `json:"createdAt"`
}

const agentCols = `a.id::text, a.name, st.name, a.status, a.version, a.last_heartbeat_at, a.credential_rotated_at, a.revoked_at,
	EXISTS (SELECT 1 FROM agent_registration_tokens t WHERE t.agent_id = a.id AND t.used_at IS NULL AND t.expires_at > now()), a.created_at`

func scanAgent(r pgx.Row) (Agent, error) {
	var a Agent
	err := r.Scan(&a.ID, &a.Name, &a.SiteName, &a.Status, &a.Version, &a.LastHeartbeatAt, &a.RotatedAt, &a.RevokedAt, &a.PendingToken, &a.CreatedAt)
	return a, err
}

func (h *Handler) agents(c *gin.Context) {
	rows, err := h.svc.conn.Query(c.Request.Context(), "SELECT "+agentCols+" FROM discovery_agents a LEFT JOIN sites st ON st.id = a.site_id ORDER BY a.name")
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Agent, error) { return scanAgent(r) })
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, httpx.Page{Number: 1, Size: len(out)}, int64(len(out)))
}

type tokenResponse struct {
	Agent     Agent     `json:"agent"`
	Token     string    `json:"registrationToken"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (h *Handler) loadAgent(c *gin.Context, tx pgx.Tx, id string) (Agent, error) {
	a, err := scanAgent(tx.QueryRow(c.Request.Context(), "SELECT "+agentCols+" FROM discovery_agents a LEFT JOIN sites st ON st.id = a.site_id WHERE a.id = $1::uuid", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return a, httpx.ErrNotFound
	}
	return a, err
}

// createAgent registers an agent and returns its one-time registration token.
// Only the token's hash is stored; it is shown once.
func (h *Handler) createAgent(c *gin.Context) {
	var req struct {
		Name   string  `json:"name"`
		SiteID *string `json:"siteId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		httpx.Fail(c, httpx.Validation("name", "Name the agent."))
		return
	}
	if req.SiteID != nil && !httpx.IsUUID(*req.SiteID) {
		req.SiteID = nil
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out tokenResponse
	err := db.InTx(ctx, h.svc.conn, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO discovery_agents (name, site_id) VALUES ($1, $2::uuid) RETURNING id::text`, req.Name, req.SiteID).Scan(&id); err != nil {
			return err
		}
		var err error
		if out.Token, out.ExpiresAt, err = IssueToken(ctx, tx, actor, id); err != nil {
			return err
		}
		if out.Agent, err = h.loadAgent(c, tx, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "discovery_agent.create", ObjectType: "discovery_agent", ObjectID: id, New: out.Agent})
	})
	switch db.ErrorCode(err) {
	case db.CodeUnique:
		err = httpx.NewError(http.StatusConflict, "ALREADY_EXISTS", "An agent with this name already exists.")
	case db.CodeForeignKey:
		err = httpx.Validation("siteId", "That site doesn't exist.")
	}
	respond(c, http.StatusCreated, out, err)
}

// rotateAgent issues a fresh registration token. The agent re-registers with
// it and gets a new credential; the old credential stops working then.
func (h *Handler) rotateAgent(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out tokenResponse
	err := db.InTx(ctx, h.svc.conn, func(tx pgx.Tx) error {
		a, err := h.loadAgent(c, tx, id)
		if err != nil {
			return err
		}
		if a.Status == "revoked" {
			return httpx.NewError(http.StatusConflict, "AGENT_REVOKED", "A revoked agent can't be rotated. Create a new agent.")
		}
		if out.Token, out.ExpiresAt, err = IssueToken(ctx, tx, actor, id); err != nil {
			return err
		}
		out.Agent = a
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "discovery_agent.rotate", ObjectType: "discovery_agent", ObjectID: id})
	})
	respond(c, http.StatusOK, out, err)
}

// revokeAgent ends an agent's access at once and hands its jobs back.
func (h *Handler) revokeAgent(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out Agent
	err := db.InTx(ctx, h.svc.conn, func(tx pgx.Tx) error {
		if _, err := h.loadAgent(c, tx, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE discovery_agents SET status = 'revoked', revoked_at = now(), credential_hash = NULL WHERE id = $1::uuid`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE agent_registration_tokens SET expires_at = now() WHERE agent_id = $1::uuid AND used_at IS NULL`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE discovery_jobs SET status = 'failed', finished_at = now(), error = 'Agent revoked' WHERE agent_id = $1::uuid AND status IN ('running','paused')`, id); err != nil {
			return err
		}
		var err error
		if out, err = h.loadAgent(c, tx, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "discovery_agent.revoke", ObjectType: "discovery_agent", ObjectID: id, New: out})
	})
	respond(c, http.StatusOK, out, err)
}
