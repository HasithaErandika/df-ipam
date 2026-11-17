// Package reports renders the Lite reports as CSV or PDF, on demand.
package reports

import (
	"fmt"
	"net/http"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/platform/tabular"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
)

// maxSpan keeps one dated report to a year, so it stays quick to run.
const maxSpan = 366 * 24 * time.Hour

type Handler struct{ conn db.Conn }

func New(conn db.Conn) *Handler { return &Handler{conn: conn} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/reports", h.list, auth.ReportsExport)
	g.Handle(http.MethodGet, "/reports/:key", h.download, auth.ReportsExport)
}

func (h *Handler) list(c *gin.Context) { httpx.OK(c, http.StatusOK, catalogue) }

func (h *Handler) download(c *gin.Context) {
	r, ok := find(c.Param("key"))
	if !ok {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	format := c.DefaultQuery("format", "csv")
	from, to, env := c.Query("from"), c.Query("to"), c.Query("environmentId")
	var v validate.V
	v.OneOf("format", format, []string{"csv", "txt", "xlsx", "pdf"}, "format")
	v.UUID("environmentId", env, false, "an environment")
	now := time.Now()
	if from == "" {
		from = now.AddDate(0, 0, -30).Format(time.DateOnly)
	}
	if to == "" {
		to = now.Format(time.DateOnly)
	}
	v.Date("from", &from, false, "From")
	v.Date("to", &to, false, "To")
	a, e1 := time.Parse(time.DateOnly, from)
	b, e2 := time.Parse(time.DateOnly, to)
	if e1 == nil && e2 == nil {
		if b.Before(a) {
			v.Add("to", "The end date is before the start date.")
		} else if r.Dated && b.Sub(a) > maxSpan {
			v.Add("to", "Choose a range of one year or less.")
		}
	}
	if err := v.Err(); err != nil {
		httpx.Fail(c, err)
		return
	}
	limit := maxRowsTable
	var out renderer
	if format != "pdf" {
		table, err := newTable(format, r)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		out = table
	} else {
		limit = maxRowsPDF
		subtitle := "Data as of " + now.Format("2 Jan 2006 15:04")
		if r.Dated {
			subtitle = fmt.Sprintf("%s to %s · %s", a.Format("2 Jan 2006"), b.Format("2 Jan 2006"), subtitle)
		}
		out = newPDF(r, subtitle, now)
	}
	body, err := h.render(c, r, out, limit, from, to, env)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	name := fmt.Sprintf("dfipam-%s-%s.%s", r.Key, now.Format("20060102"), format)
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Header("Cache-Control", "no-store")
	contentType := tabular.Formats[format]
	if format == "pdf" {
		contentType = "application/pdf"
	}
	c.Data(http.StatusOK, contentType, body)
}

// render runs the report query and feeds each row to the renderer, stopping one
// row past the limit so it can say the file was cut short.
func (h *Handler) render(c *gin.Context, r Report, out renderer, limit int, from, to, env string) ([]byte, error) {
	rows, err := h.conn.Query(c, r.sql+fmt.Sprintf(" LIMIT %d", limit+1), from, to, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cells := make([]string, len(r.Columns))
	dest := make([]any, len(cells))
	for i := range cells {
		dest[i] = &cells[i]
	}
	n := 0
	for rows.Next() {
		if n++; n > limit {
			break
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		if err := out.row(append([]string(nil), cells...)); err != nil {
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out.finish(n > limit, limit)
}
