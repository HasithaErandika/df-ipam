package inventory

import (
	"net/http"

	"dfipam/api/internal/ipam"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
)

// candidate suggests the next Available candidate in a segment.
func (h *Handler) candidate(c *gin.Context) {
	segmentID := c.Query("segmentId")
	var v validate.V
	v.UUID("segmentId", segmentID, true, "a segment")
	if err := v.Err(); err != nil {
		httpx.Fail(c, err)
		return
	}
	ip, err := ipam.Candidate(c, h.conn, segmentID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"ip": ip, "label": "Available candidate"})
}

// check says what is already true of one address, so a form can warn before saving.
func (h *Handler) check(c *gin.Context) {
	segmentID, raw := c.Query("segmentId"), c.Query("ip")
	var v validate.V
	v.UUID("segmentId", segmentID, true, "a segment")
	ip := v.IPv4("ip", &raw, true)
	if err := v.Err(); err != nil {
		httpx.Fail(c, err)
		return
	}
	status, err := ipam.Check(c, h.conn, segmentID, ip)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"status": status, "available": status.Available()})
}
