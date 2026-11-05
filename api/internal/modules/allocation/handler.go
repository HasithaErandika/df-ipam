package allocation

import (
	"net/http"
	"strconv"
	"strings"

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

func (h *Handler) listRequests(c *gin.Context) {
	p := httpx.ParsePage(c)
	rows, total, err := h.svc.ListRequests(c.Request.Context(), auth.Current(c), RequestFilter{
		Status: c.Query("status"), Type: c.Query("type"), SubnetID: c.Query("subnetId"),
		Query: strings.TrimSpace(c.Query("q")), Mine: c.Query("mine") == "true",
	}, p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, rows, p, total)
}

func (h *Handler) getRequest(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	r, err := h.svc.GetRequest(c.Request.Context(), auth.Current(c), id)
	respond(c, http.StatusOK, r, err)
}

func (h *Handler) createRequest(c *gin.Context) {
	var in RequestInput
	if !bind(c, &in) {
		return
	}
	r, err := h.svc.CreateRequest(c.Request.Context(), auth.Current(c), in)
	respond(c, http.StatusCreated, r, err)
}

func (h *Handler) updateRequest(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	version, err := strconv.ParseInt(strings.Trim(c.GetHeader("If-Match"), `"`), 10, 32)
	if err != nil {
		httpx.Fail(c, httpx.NewError(http.StatusPreconditionFailed, "STALE_VERSION", "Send the record version in If-Match."))
		return
	}
	var in RequestInput
	if !bind(c, &in) {
		return
	}
	r, err := h.svc.UpdateRequest(c.Request.Context(), auth.Current(c), id, int32(version), in)
	respond(c, http.StatusOK, r, err)
}

func (h *Handler) deleteRequest(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteRequest(c.Request.Context(), auth.Current(c), id); err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) submit(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	r, err := h.svc.Submit(c.Request.Context(), auth.Current(c), id)
	respond(c, http.StatusOK, r, err)
}

func (h *Handler) cancel(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	r, err := h.svc.Cancel(c.Request.Context(), auth.Current(c), id)
	respond(c, http.StatusOK, r, err)
}

func (h *Handler) pending(c *gin.Context) {
	p := httpx.ParsePage(c)
	rows, total, err := h.svc.Pending(c.Request.Context(), auth.Current(c), p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, rows, p, total)
}

func (h *Handler) decide(c *gin.Context, approve bool) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var body struct {
		Comment string `json:"comment"`
	}
	if c.Request.ContentLength != 0 && !bind(c, &body) {
		return
	}
	r, err := h.svc.Decide(c.Request.Context(), auth.Current(c), id, approve, body.Comment)
	respond(c, http.StatusOK, r, err)
}

func (h *Handler) approve(c *gin.Context) { h.decide(c, true) }
func (h *Handler) reject(c *gin.Context)  { h.decide(c, false) }

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

func listFilter(c *gin.Context) ListFilter {
	days, _ := strconv.Atoi(c.Query("expiringWithinDays"))
	if days < 0 || days > MaxReservationDays {
		days = 0
	}
	return ListFilter{Status: c.Query("status"), SubnetID: c.Query("subnetId"), RoutingDomainID: c.Query("routingDomainId"),
		Query: strings.TrimSpace(c.Query("q")), ExpiringWithinDays: days}
}

func (h *Handler) listAllocations(c *gin.Context) {
	p := httpx.ParsePage(c)
	rows, total, err := h.svc.ListAllocations(c.Request.Context(), listFilter(c), p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, rows, p, total)
}

func (h *Handler) getAllocation(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	a, err := h.svc.GetAllocation(c.Request.Context(), id)
	respond(c, http.StatusOK, a, err)
}

type fulfil struct {
	RequestID string `json:"requestId"`
}

func (f fulfil) valid(c *gin.Context) bool {
	if !httpx.IsUUID(f.RequestID) {
		httpx.Fail(c, httpx.Validation("requestId", "Choose an approved request."))
		return false
	}
	return true
}

// allocate fulfils a request. Repeating it returns the same allocation with 200,
// which is what makes the call safe to retry.
func (h *Handler) allocate(c *gin.Context) {
	var body fulfil
	if !bind(c, &body) || !body.valid(c) {
		return
	}
	a, created, err := h.svc.Allocate(c.Request.Context(), auth.Current(c), body.RequestID)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	respond(c, status, a, err)
}

func (h *Handler) release(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var body fulfil
	if !bind(c, &body) || !body.valid(c) {
		return
	}
	a, err := h.svc.Release(c.Request.Context(), auth.Current(c), id, body.RequestID)
	respond(c, http.StatusOK, a, err)
}

func (h *Handler) listReservations(c *gin.Context) {
	p := httpx.ParsePage(c)
	rows, total, err := h.svc.ListReservations(c.Request.Context(), listFilter(c), p)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, rows, p, total)
}

func (h *Handler) getReservation(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	r, err := h.svc.GetReservation(c.Request.Context(), id)
	respond(c, http.StatusOK, r, err)
}

func (h *Handler) reserve(c *gin.Context) {
	var body fulfil
	if !bind(c, &body) || !body.valid(c) {
		return
	}
	r, created, err := h.svc.Reserve(c.Request.Context(), auth.Current(c), body.RequestID)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	respond(c, status, r, err)
}

func (h *Handler) extend(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var body struct {
		ExpiryDate string `json:"expiryDate"`
		Reason     string `json:"reason"`
	}
	if !bind(c, &body) {
		return
	}
	r, err := h.svc.Extend(c.Request.Context(), auth.Current(c), id, body.ExpiryDate, body.Reason)
	respond(c, http.StatusOK, r, err)
}

func (h *Handler) releaseReservation(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if !bind(c, &body) {
		return
	}
	r, err := h.svc.ReleaseReservation(c.Request.Context(), auth.Current(c), id, body.Reason)
	respond(c, http.StatusOK, r, err)
}
