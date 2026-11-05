// Package dashboard serves summary counts.
package dashboard

import (
	"net/http"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

type Summary struct {
	SubnetsV4          int64 `json:"subnetsV4"`
	SubnetsV6          int64 `json:"subnetsV6"`
	IPRecords          int64 `json:"ipRecords"`
	Allocated          int64 `json:"allocated"`
	Reserved           int64 `json:"reserved"`
	AvailableCandidate int64 `json:"availableCandidates"`
	ObservedActive     int64 `json:"observedActive"`
	UnknownObservation int64 `json:"unknownObservation"`
	ConflictSuspected  int64 `json:"conflictSuspected"`
	Devices            int64 `json:"devices"`
	Quarantined        int64 `json:"quarantined"`
	OpenFindings       int64 `json:"openFindings"`
}

type Handler struct{ pool db.Conn }

func NewHandler(pool db.Conn) *Handler { return &Handler{pool} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/dashboard/summary", h.summary, auth.InventoryView)
}

func (h *Handler) summary(c *gin.Context) {
	var s Summary
	err := h.pool.QueryRow(c.Request.Context(), `
		SELECT
		  (SELECT count(*) FROM subnets WHERE archived_at IS NULL AND family(cidr) = 4),
		  (SELECT count(*) FROM subnets WHERE archived_at IS NULL AND family(cidr) = 6),
		  count(*),
		  count(*) FILTER (WHERE allocation_state = 'allocated'),
		  count(*) FILTER (WHERE allocation_state = 'reserved'),
		  count(*) FILTER (WHERE allocation_state = 'available_candidate'),
		  count(*) FILTER (WHERE observation_state = 'observed_active'),
		  count(*) FILTER (WHERE observation_state = 'unknown'),
		  count(*) FILTER (WHERE observation_state = 'conflict_suspected'),
		  (SELECT count(*) FROM devices),
		  count(*) FILTER (WHERE allocation_state = 'quarantined'),
		  (SELECT count(*) FROM reconciliation_results WHERE review_status IN ('open', 'investigating'))
		FROM ip_addresses`).Scan(&s.SubnetsV4, &s.SubnetsV6, &s.IPRecords, &s.Allocated, &s.Reserved,
		&s.AvailableCandidate, &s.ObservedActive, &s.UnknownObservation, &s.ConflictSuspected, &s.Devices, &s.Quarantined, &s.OpenFindings)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, s)
}
