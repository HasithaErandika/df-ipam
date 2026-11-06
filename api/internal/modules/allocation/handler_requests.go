package allocation

import (
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/httpx"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

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

func (h *Handler) reject(c *gin.Context) { h.decide(c, false) }
