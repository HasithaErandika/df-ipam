package allocation

import (
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/httpx"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

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
