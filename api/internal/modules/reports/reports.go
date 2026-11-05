package reports

import (
	"encoding/json"
	"net/http"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const (
	maxActivePerUser = 5
	jobTimeout       = 10 * time.Minute
	stuckAfter       = 15 // minutes
)

var (
	errBusy    = httpx.NewError(http.StatusTooManyRequests, "REPORT_LIMIT", "You already have several reports running. Wait for one to finish.")
	errNotDone = httpx.NewError(http.StatusConflict, "REPORT_NOT_READY", "This report isn't ready to download.")
)

// Job is a report's status. The file itself is only sent by download.
type Job struct {
	ID         string     `json:"id"`
	ReportType string     `json:"reportType"`
	Title      string     `json:"title"`
	Format     string     `json:"format"`
	Filters    Filters    `json:"filters"`
	Status     string     `json:"status"`
	Error      string     `json:"error"`
	RowCount   int64      `json:"rowCount"`
	SizeBytes  int64      `json:"sizeBytes"`
	Truncated  bool       `json:"truncated"`
	DataAsOf   *time.Time `json:"dataAsOf"`
	CreatedAt  time.Time  `json:"createdAt"`
	FinishedAt *time.Time `json:"finishedAt"`
}

const jobColumns = `id::text, report_type, format, filters, status, error, row_count, size_bytes, truncated, data_as_of, created_at, finished_at`

func scanJob(row pgx.Row) (Job, error) {
	var j Job
	var filters []byte
	if err := row.Scan(&j.ID, &j.ReportType, &j.Format, &filters, &j.Status, &j.Error, &j.RowCount, &j.SizeBytes, &j.Truncated, &j.DataAsOf, &j.CreatedAt, &j.FinishedAt); err != nil {
		return j, err
	}
	if d, ok := definition(j.ReportType); ok {
		j.Title = d.Title
	}
	if err := json.Unmarshal(filters, &j.Filters); err != nil {
		return j, err
	}
	j.Filters.Scope = nil // who may see what isn't part of the response
	return j, nil
}

// Handler serves reports.
type Handler struct{ conn db.Conn }

// NewHandler returns a handler.
func NewHandler(conn db.Conn) *Handler { return &Handler{conn} }

// Register adds the routes.
func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/reports/types", h.types, auth.ReportsExport, auth.ReportsExportScoped)
	g.Handle(http.MethodGet, "/reports/summary", h.summary, auth.ReportsExport, auth.ReportsExportScoped)
	g.Handle(http.MethodGet, "/reports", h.list, auth.ReportsExport, auth.ReportsExportScoped)
	g.Handle(http.MethodPost, "/reports", h.create, auth.ReportsExport, auth.ReportsExportScoped)
	g.Handle(http.MethodGet, "/reports/:id/download", h.download, auth.ReportsExport, auth.ReportsExportScoped)
}

func (h *Handler) types(c *gin.Context) {
	p := auth.Current(c)
	out := []Definition{}
	for _, d := range Definitions {
		if _, ok := allowed(p, d); ok {
			out = append(out, d)
		}
	}
	httpx.OK(c, http.StatusOK, out)
}

// Summary is the small overview at the top of the Reports page. Inventory is left
// out for scoped people, because whole-estate totals go beyond what they may see.
type Summary struct {
	Total      int64      `json:"total"`
	Ready      int64      `json:"ready"`
	InProgress int64      `json:"inProgress"`
	Failed     int64      `json:"failed"`
	LastReady  *time.Time `json:"lastReadyAt"`
	Inventory  *Inventory `json:"inventory"`
}

// Inventory counts what the registers currently cover.
type Inventory struct {
	Subnets      int64 `json:"subnets"`
	IPRecords    int64 `json:"ipRecords"`
	Allocated    int64 `json:"allocated"`
	Reserved     int64 `json:"reserved"`
	Devices      int64 `json:"devices"`
	OpenFindings int64 `json:"openFindings"`
}

func (h *Handler) summary(c *gin.Context) {
	ctx, p := c.Request.Context(), auth.Current(c)
	var out Summary
	err := h.conn.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE status = 'completed'), count(*) FILTER (WHERE status IN ('queued', 'running')),
		  count(*) FILTER (WHERE status = 'failed'), max(finished_at) FILTER (WHERE status = 'completed')
		FROM report_jobs WHERE requested_by = $1::uuid`, p.UserID).Scan(&out.Total, &out.Ready, &out.InProgress, &out.Failed, &out.LastReady)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if p.Has(auth.ReportsExport) {
		var v Inventory
		err := h.conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM subnets WHERE archived_at IS NULL), count(*),
			  count(*) FILTER (WHERE allocation_state = 'allocated'), count(*) FILTER (WHERE allocation_state = 'reserved'),
			  (SELECT count(*) FROM devices), (SELECT count(*) FROM reconciliation_results WHERE review_status IN ('open', 'investigating'))
			FROM ip_addresses`).Scan(&v.Subnets, &v.IPRecords, &v.Allocated, &v.Reserved, &v.Devices, &v.OpenFindings)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		out.Inventory = &v
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) list(c *gin.Context) {
	p := httpx.ParsePage(c)
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var total int64
	if err := h.conn.QueryRow(ctx, `SELECT count(*) FROM report_jobs WHERE requested_by = $1::uuid`, actor).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := h.conn.Query(ctx, `SELECT `+jobColumns+` FROM report_jobs WHERE requested_by = $1::uuid ORDER BY created_at DESC, id LIMIT $2 OFFSET $3`, actor, p.Size, p.Offset())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Job, error) { return scanJob(r) })
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}
