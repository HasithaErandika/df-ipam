package discovery

import (
	"net/http"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
)

// summaryDTO counts the findings in the chosen segment and time window, so the
// screen can show where attention is needed before the table.
type summaryDTO struct {
	Total    int64 `json:"total"`
	Needs    int64 `json:"needs"`
	Unknown  int64 `json:"unknown"`
	Alerting int64 `json:"alerting"`
	Known    int64 `json:"known"`
	New      int64 `json:"new"`
}

const summarySQL = `SELECT count(*), count(*) FILTER (WHERE ` + alerting + ` OR ` + undecided + `),
	count(*) FILTER (WHERE ` + undecided + `), count(*) FILTER (WHERE ` + alerting + `),
	count(*) FILTER (WHERE ` + known + `),
	count(*) FILTER (WHERE v.first_seen > now() - ` + newWindow + `)` + findingFrom

// summary takes only the segment and the time window: the other filters narrow
// the table, while these counts are what the chips switch between.
func (h *Handler) summary(c *gin.Context) {
	var w db.Where
	var v validate.V
	if id := c.Query("segmentId"); id != "" {
		v.UUID("segmentId", id, true, "a segment")
		w.Add("v.segment_id = ?::uuid", id)
	}
	seenFilter(c.DefaultQuery("seen", "now"), &w, &v)
	if err := v.Err(); err != nil {
		httpx.Fail(c, err)
		return
	}
	var out summaryDTO
	if err := h.conn.QueryRow(c, summarySQL+w.SQL(), w.Args...).Scan(&out.Total, &out.Needs, &out.Unknown, &out.Alerting, &out.Known, &out.New); err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}
