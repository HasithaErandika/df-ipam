// Package reports renders the Lite reports as CSV or PDF, on demand.
package reports

import (
	"fmt"
	"net/http"
	"time"

	"dfipam/api/internal/audit"
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

// allowed says whether this person holds what the report also needs.
func allowed(c *gin.Context, r Report) bool {
	for _, p := range r.Needs {
		if !auth.Current(c).Has(p) {
			return false
		}
	}
	return true
}

func (h *Handler) list(c *gin.Context) {
	out := []Report{}
	for _, r := range all() {
		if allowed(c, r) {
			out = append(out, r)
		}
	}
	httpx.OK(c, http.StatusOK, out)
}

var colombo = time.FixedZone("Asia/Colombo", 5*3600+1800)

func (h *Handler) download(c *gin.Context) {
	r, ok := find(c.Param("key"))
	if !ok {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	if !allowed(c, r) {
		httpx.Fail(c, httpx.ErrForbidden)
		return
	}
	q, err := readQuery(c, r)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	format, from, to, env, now := q.format, q.from, q.to, q.env, q.now
	out, limit, err := q.renderer(r)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	body, err := h.render(c, r, out, limit, from, to, env)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if err := audit.Export(c, h.conn, auth.Current(c).UserID, "report:"+r.Key, format, c.Request.URL.Query()); err != nil {
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

// query is a checked download request. Dates are filled in when left out.
type query struct {
	format, from, to, env string
	a, b, now             time.Time
}

func readQuery(c *gin.Context, r Report) (query, error) {
	q := query{format: c.DefaultQuery("format", "csv"), from: c.Query("from"), to: c.Query("to"), env: c.Query("environmentId")}
	var v validate.V
	v.OneOf("format", q.format, []string{"csv", "txt", "xlsx", "pdf"}, "format")
	v.UUID("environmentId", q.env, false, "an environment")
	q.now = time.Now().In(colombo) // default dates are Sri Lankan days, whatever the server's zone
	if q.from == "" {
		q.from = q.now.AddDate(0, 0, -30).Format(time.DateOnly)
	}
	if q.to == "" {
		q.to = q.now.Format(time.DateOnly)
	}
	v.Date("from", &q.from, false, "From")
	v.Date("to", &q.to, false, "To")
	var e1, e2 error
	q.a, e1 = time.Parse(time.DateOnly, q.from)
	q.b, e2 = time.Parse(time.DateOnly, q.to)
	if e1 == nil && e2 == nil {
		if q.b.Before(q.a) {
			v.Add("to", "The end date is before the start date.")
		} else if r.Dated && q.b.Sub(q.a) > maxSpan {
			v.Add("to", "Choose a range of one year or less.")
		}
	}
	return q, v.Err()
}

// renderer picks the output for the format and how many rows it may hold.
func (q query) renderer(r Report) (renderer, int, error) {
	if q.format != "pdf" {
		table, err := newTable(q.format, r)
		if err != nil {
			return nil, 0, err
		}
		return table, maxRowsTable, nil
	}
	subtitle := "Data as of " + q.now.Format("2 Jan 2006 15:04")
	if r.Dated {
		subtitle = fmt.Sprintf("%s to %s · %s", q.a.Format("2 Jan 2006"), q.b.Format("2 Jan 2006"), subtitle)
	}
	return newPDF(r, subtitle, q.now), maxRowsPDF, nil
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
