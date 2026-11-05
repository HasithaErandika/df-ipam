package discovery

import (
	"net/http"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
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
		Paused        int64      `json:"paused"`
		AgentsOnline  int64      `json:"agentsOnline"`
		AgentsTotal   int64      `json:"agentsTotal"`
	}
	err := h.svc.conn.QueryRow(ctx, `SELECT
		coalesce((SELECT value = 'true'::jsonb FROM system_settings WHERE key = 'discovery.emergency_stop'), false),
		(SELECT updated_at FROM system_settings WHERE key = 'discovery.emergency_stop'),
		(SELECT count(*) FROM discovery_jobs WHERE status = 'queued'), (SELECT count(*) FROM discovery_jobs WHERE status = 'running'),
		(SELECT count(*) FROM discovery_jobs WHERE status = 'paused'),
		(SELECT count(*) FROM discovery_agents WHERE status = 'active'), (SELECT count(*) FROM discovery_agents WHERE status <> 'revoked')`).
		Scan(&out.EmergencyStop, &out.StoppedAt, &out.Queued, &out.Running, &out.Paused, &out.AgentsOnline, &out.AgentsTotal)
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
