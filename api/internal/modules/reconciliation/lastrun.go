package reconciliation

import (
	"errors"
	"net/http"
	"time"

	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// lastRunInfo says when reconciliation last ran. RanBy is empty for the hourly run.
type lastRunInfo struct {
	RanAt    *time.Time `json:"ranAt"`
	RanBy    *string    `json:"ranBy"`
	New      int64      `json:"new"`
	Resolved int64      `json:"resolved"`
	Open     int64      `json:"open"`
}

// lastRun reads the audit log, which records every run, manual or automatic.
func (h *Handler) lastRun(c *gin.Context) {
	var out lastRunInfo
	err := h.conn.QueryRow(c.Request.Context(), `SELECT a.occurred_at, u.email, coalesce((a.new_values->>'new')::bigint, 0),
		  coalesce((a.new_values->>'resolved')::bigint, 0), coalesce((a.new_values->>'open')::bigint, 0)
		FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id
		WHERE a.action = 'reconciliation.run' ORDER BY a.occurred_at DESC, a.id DESC LIMIT 1`).
		Scan(&out.RanAt, &out.RanBy, &out.New, &out.Resolved, &out.Open)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}
