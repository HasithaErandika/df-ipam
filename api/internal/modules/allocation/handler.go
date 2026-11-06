package allocation

import (
	"net/http"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc} }

// viewRequests may read requests. Without one of the wider permissions, a
// caller sees only their own.
var viewRequests = []string{auth.IPRequest, auth.RequestsApprove, auth.IPAllocate, auth.IPReserve, auth.AuditView}

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/requests", h.listRequests, viewRequests...)
	g.Handle(http.MethodPost, "/requests", h.createRequest, auth.IPRequest)
	g.Handle(http.MethodGet, "/requests/:id", h.getRequest, viewRequests...)
	g.Handle(http.MethodPatch, "/requests/:id", h.updateRequest, auth.IPRequest)
	g.Handle(http.MethodDelete, "/requests/:id", h.deleteRequest, auth.IPRequest)
	g.Handle(http.MethodPost, "/requests/:id/submit", h.submit, auth.IPRequest)
	g.Handle(http.MethodPost, "/requests/:id/cancel", h.cancel, auth.IPRequest)

	g.Handle(http.MethodGet, "/approvals", h.pending, auth.RequestsApprove)
	g.Handle(http.MethodPost, "/approvals/:id/approve", h.approve, auth.RequestsApprove)
	g.Handle(http.MethodPost, "/approvals/:id/reject", h.reject, auth.RequestsApprove)

	g.Handle(http.MethodGet, "/approval-workflows", h.listWorkflows, auth.RequestsApprove, auth.UsersManage)
	g.Handle(http.MethodPost, "/approval-workflows", h.createWorkflow, auth.UsersManage)
	g.Handle(http.MethodPut, "/approval-workflows/:id", h.updateWorkflow, auth.UsersManage)
	g.Handle(http.MethodDelete, "/approval-workflows/:id", h.deleteWorkflow, auth.UsersManage)

	g.Handle(http.MethodGet, "/allocations", h.listAllocations, auth.InventoryView)
	g.Handle(http.MethodPost, "/allocations", h.allocate, auth.IPAllocate)
	g.Handle(http.MethodGet, "/allocations/:id", h.getAllocation, auth.InventoryView)
	g.Handle(http.MethodPost, "/allocations/:id/release", h.release, auth.IPAllocate)

	g.Handle(http.MethodGet, "/reservations", h.listReservations, auth.InventoryView)
	g.Handle(http.MethodPost, "/reservations", h.reserve, auth.IPReserve)
	g.Handle(http.MethodGet, "/reservations/:id", h.getReservation, auth.InventoryView)
	g.Handle(http.MethodPost, "/reservations/:id/extend", h.extend, auth.IPReserve)
	g.Handle(http.MethodPost, "/reservations/:id/release", h.releaseReservation, auth.IPReserve)
}

func bind(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return false
	}
	return true
}

func respond(c *gin.Context, status int, v any, err error) {
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, status, v)
}

func (h *Handler) listWorkflows(c *gin.Context) {
	rows, err := h.svc.ListWorkflows(c.Request.Context())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, rows, httpx.Page{Number: 1, Size: len(rows)}, int64(len(rows)))
}

func (h *Handler) createWorkflow(c *gin.Context) {
	var in WorkflowInput
	if !bind(c, &in) {
		return
	}
	w, err := h.svc.CreateWorkflow(c.Request.Context(), auth.Current(c), in)
	respond(c, http.StatusCreated, w, err)
}

func (h *Handler) updateWorkflow(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var in WorkflowInput
	if !bind(c, &in) {
		return
	}
	w, err := h.svc.UpdateWorkflow(c.Request.Context(), auth.Current(c), id, in)
	respond(c, http.StatusOK, w, err)
}

func (h *Handler) deleteWorkflow(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteWorkflow(c.Request.Context(), auth.Current(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
