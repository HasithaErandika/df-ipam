package integrations

import (
	"net/http"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

// Handler serves the Integrations API.
type Handler struct{ svc *Service }

// NewHandler returns a handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc} }

// Register adds the routes. Viewing needs the same trust as managing, or the audit
// role: connection details describe the bank's infrastructure.
func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/integrations/kinds", h.kinds, auth.IntegrationsManage, auth.AuditView)
	g.Handle(http.MethodGet, "/integrations/connections", h.list, auth.IntegrationsManage, auth.AuditView)
	g.Handle(http.MethodPost, "/integrations/connections", h.create, auth.IntegrationsManage)
	g.Handle(http.MethodGet, "/integrations/connections/:id", h.get, auth.IntegrationsManage, auth.AuditView)
	g.Handle(http.MethodPut, "/integrations/connections/:id", h.update, auth.IntegrationsManage)
	g.Handle(http.MethodDelete, "/integrations/connections/:id", h.delete, auth.IntegrationsManage)
	g.Handle(http.MethodPost, "/integrations/connections/:id/test", h.test, auth.IntegrationsManage)
	g.Handle(http.MethodPost, "/integrations/connections/:id/sync", h.sync, auth.IntegrationsManage)
	g.Handle(http.MethodGet, "/integrations/connections/:id/syncs", h.syncs, auth.IntegrationsManage, auth.AuditView)
	g.Handle(http.MethodGet, "/integrations/syncs/:id/errors", h.errors, auth.IntegrationsManage, auth.AuditView)
}

func (h *Handler) kinds(c *gin.Context) { httpx.OK(c, http.StatusOK, Kinds) }

func (h *Handler) list(c *gin.Context) {
	p := httpx.ParsePage(c)
	out, total, err := h.svc.List(c.Request.Context(), p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}

func (h *Handler) get(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	out, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) create(c *gin.Context) {
	var in Input
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	out, err := h.svc.Create(c.Request.Context(), auth.Current(c).UserID, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, out)
}

func (h *Handler) update(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var in Input
	if err := c.ShouldBindJSON(&in); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	out, err := h.svc.Update(c.Request.Context(), auth.Current(c).UserID, id, in)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) delete(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), auth.Current(c).UserID, id); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) test(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	out, err := h.svc.Test(c.Request.Context(), auth.Current(c).UserID, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) sync(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	out, err := h.svc.RequestSync(c.Request.Context(), auth.Current(c).UserID, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusAccepted, out)
}

func (h *Handler) syncs(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	p := httpx.ParsePage(c)
	out, total, err := h.svc.Syncs(c.Request.Context(), id, p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}

func (h *Handler) errors(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	p := httpx.ParsePage(c)
	out, total, err := h.svc.Errors(c.Request.Context(), id, p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}
