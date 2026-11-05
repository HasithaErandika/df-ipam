package reports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/notifications"
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

// Filters are the choices a report accepts. Scope is set by the server, never read
// from a request: a nil scope means the requester has full export rights.
type Filters struct {
	RoutingDomainID string    `json:"routingDomainId,omitempty"`
	Environment     string    `json:"environment,omitempty"`
	From            string    `json:"from,omitempty"`
	To              string    `json:"to,omitempty"`
	Scope           *[]string `json:"scope,omitempty"`
}

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

// allowed reports whether a person may run a report, and with what scope. Full
// export rights mean no scope.
func allowed(p *auth.Principal, d Definition) (full, ok bool) {
	if d.Permission == auth.AuditView {
		return true, p.Has(auth.AuditView)
	}
	if p.Has(auth.ReportsExport) {
		return true, true
	}
	return false, d.Scoped && p.Has(auth.ReportsExportScoped)
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

var environments = []string{"production", "disaster_recovery", "uat", "development", "testing"}

func parseDay(field, v string) error {
	if v == "" {
		return nil
	}
	if _, err := time.Parse("2006-01-02", v); err != nil {
		return httpx.Validation(field, "Use a date in the form YYYY-MM-DD.")
	}
	return nil
}

func (f Filters) validate(d Definition) error {
	used := map[string]string{"routingDomainId": f.RoutingDomainID, "environment": f.Environment, "from": f.From, "to": f.To}
	for name, v := range used {
		if v != "" && !slices.Contains(d.Filters, name) {
			return httpx.Validation(name, "This report doesn't use this filter.")
		}
	}
	if f.RoutingDomainID != "" && !httpx.IsUUID(f.RoutingDomainID) {
		return httpx.Validation("routingDomainId", "Choose a valid routing domain.")
	}
	if f.Environment != "" && !slices.Contains(environments, f.Environment) {
		return httpx.Validation("environment", "Choose a valid environment.")
	}
	if err := parseDay("from", f.From); err != nil {
		return err
	}
	if err := parseDay("to", f.To); err != nil {
		return err
	}
	if f.From != "" && f.To != "" && f.To < f.From {
		return httpx.Validation("to", "The end date is before the start date.")
	}
	return nil
}

// scopeOf lists the business services a scoped role grants. Empty means none, so a
// scoped report comes back empty rather than unfiltered.
func scopeOf(ctx context.Context, q db.Conn, user string) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT DISTINCT ur.business_service_id::text FROM user_roles ur
		JOIN role_permissions rp ON rp.role_id = ur.role_id JOIN permissions pm ON pm.id = rp.permission_id
		WHERE ur.user_id = $1::uuid AND ur.business_service_id IS NOT NULL AND pm.resource = 'reports' AND pm.action = 'export_scoped'`, user)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if out == nil {
		out = []string{}
	}
	return out, err
}

func (h *Handler) create(c *gin.Context) {
	var req struct {
		ReportType string  `json:"reportType"`
		Format     string  `json:"format"`
		Filters    Filters `json:"filters"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	d, known := definition(req.ReportType)
	if !known {
		httpx.Fail(c, httpx.Validation("reportType", "Choose a valid report."))
		return
	}
	if _, ok := formats[req.Format]; !ok {
		httpx.Fail(c, httpx.Validation("format", "Choose Excel, CSV or PDF."))
		return
	}
	req.Filters.Scope = nil
	if err := req.Filters.validate(d); err != nil {
		httpx.Fail(c, err)
		return
	}
	principal := auth.Current(c)
	full, ok := allowed(principal, d)
	if !ok {
		httpx.Fail(c, httpx.ErrForbidden)
		return
	}
	ctx, actor := c.Request.Context(), principal.UserID
	var out Job
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		// Serialises one person's requests, so the limit can't be raced past.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 7))`, actor); err != nil {
			return err
		}
		var active int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM report_jobs WHERE requested_by = $1::uuid AND status IN ('queued', 'running')`, actor).Scan(&active); err != nil {
			return err
		}
		if active >= maxActivePerUser {
			return errBusy
		}
		if !full {
			scope, err := scopeOf(ctx, tx, actor)
			if err != nil {
				return err
			}
			req.Filters.Scope = &scope
		}
		raw, err := json.Marshal(req.Filters)
		if err != nil {
			return err
		}
		out, err = scanJob(tx.QueryRow(ctx, `INSERT INTO report_jobs (report_type, format, filters, requested_by) VALUES ($1, $2, $3::jsonb, $4::uuid) RETURNING `+jobColumns,
			req.ReportType, req.Format, raw, actor))
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "report.request", ObjectType: "report", ObjectID: out.ID, New: req})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusAccepted, out)
}

// download sends a finished report to the person who requested it. A report holds a
// map of the network, so sharing it means asking the system for your own copy.
func (h *Handler) download(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var kind, format, status string
	var content []byte
	var finished *time.Time
	err := h.conn.QueryRow(ctx, `SELECT report_type, format, status, content, finished_at FROM report_jobs WHERE id = $1::uuid AND requested_by = $2::uuid`, id, actor).
		Scan(&kind, &format, &status, &content, &finished)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	if status != "completed" || content == nil {
		httpx.Fail(c, errNotDone)
		return
	}
	// Rights can be withdrawn after a report is generated.
	d, known := definition(kind)
	if !known {
		httpx.Fail(c, httpx.ErrNotFound)
		return
	}
	if _, ok := allowed(auth.Current(c), d); !ok {
		httpx.Fail(c, httpx.ErrForbidden)
		return
	}
	err = db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "report.download", ObjectType: "report", ObjectID: id, New: gin.H{"reportType": kind, "format": format}})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	f := formats[format]
	date := time.Now().UTC().Format("2006-01-02")
	if finished != nil {
		date = finished.UTC().Format("2006-01-02")
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="df-ipam-%s-%s.%s"`, strings.ReplaceAll(kind, "_", "-"), date, f.extension))
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, f.contentType, content)
}

func nullable(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// endOfDay makes an end date inclusive of that whole day.
func endOfDay(v string) *string {
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil
	}
	s := t.AddDate(0, 0, 1).Format("2006-01-02")
	return &s
}

// generate streams the query into a file. It stops at the format's row limit.
func generate(ctx context.Context, conn db.Conn, d Definition, format string, f Filters, asOf time.Time) ([]byte, int64, bool, error) {
	out, err := newRenderer(format, d, asOf)
	if err != nil {
		return nil, 0, false, err
	}
	var scope any
	if f.Scope != nil {
		scope = *f.Scope
	}
	limit := limitFor(format)
	var count int64
	var truncated bool
	// A report changes nothing, so its transaction always rolls back. That also
	// clears the read-only setting when this runs inside another transaction.
	errFinished := errors.New("report finished")
	err = db.InTx(ctx, conn, func(tx pgx.Tx) error {
		// Reports only read, and the database holds them to that.
		if _, err := tx.Exec(ctx, `SET TRANSACTION READ ONLY`); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SET LOCAL statement_timeout = '9min'`); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, d.sql+fmt.Sprintf(" LIMIT %d", limit+1), nullable(f.RoutingDomainID), nullable(f.Environment), nullable(f.From), endOfDay(f.To), scope)
		if err != nil {
			return err
		}
		defer rows.Close()
		cells := make([]string, len(d.Columns))
		dest := make([]any, len(cells))
		for i := range cells {
			dest[i] = &cells[i]
		}
		for rows.Next() {
			if count == limit {
				truncated = true
				break
			}
			if err := rows.Scan(dest...); err != nil {
				return err
			}
			if err := out.row(cells); err != nil {
				return err
			}
			count++
		}
		if !truncated {
			if err := rows.Err(); err != nil {
				return err
			}
		}
		return errFinished
	})
	if !errors.Is(err, errFinished) {
		return nil, 0, false, err
	}
	content, err := out.finish(truncated, limit)
	if err != nil {
		return nil, 0, false, err
	}
	if len(content) > maxFileBytes {
		return nil, 0, false, errTooLarge
	}
	return content, count, truncated, nil
}

// RunResult summarises one pass of the report worker.
type RunResult struct{ Completed, Failed, Stuck int64 }

// Run fails stuck jobs and works through the queue. SKIP LOCKED lets several
// workers share it.
func Run(ctx context.Context, conn db.Conn) (RunResult, error) {
	var out RunResult
	tag, err := conn.Exec(ctx, `UPDATE report_jobs SET status = 'failed', error = 'The worker stopped before this report finished.', finished_at = now()
		WHERE status = 'running' AND started_at < now() - make_interval(mins => $1)`, stuckAfter)
	if err != nil {
		return out, err
	}
	out.Stuck = tag.RowsAffected()
	for ctx.Err() == nil {
		var id, kind, format, user string
		var raw []byte
		err := conn.QueryRow(ctx, `UPDATE report_jobs SET status = 'running', started_at = now()
			WHERE id = (SELECT id FROM report_jobs WHERE status = 'queued' ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1)
			RETURNING id::text, report_type, format, filters, requested_by::text`).Scan(&id, &kind, &format, &raw, &user)
		if errors.Is(err, pgx.ErrNoRows) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		if err := runOne(ctx, conn, id, kind, format, raw, user); err != nil {
			out.Failed++
			slog.Error("report failed", "job", id, "type", kind, "error", err)
			message := "The report could not be generated."
			if errors.Is(err, errTooLarge) {
				message = "The report is too large. Narrow the filters and try again."
			}
			fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			_, uerr := conn.Exec(fctx, `UPDATE report_jobs SET status = 'failed', error = $2, finished_at = now() WHERE id = $1::uuid`, id, message)
			cancel()
			if uerr != nil {
				return out, uerr
			}
			continue
		}
		out.Completed++
	}
	return out, ctx.Err()
}

func runOne(ctx context.Context, conn db.Conn, id, kind, format string, raw []byte, user string) error {
	d, ok := definition(kind)
	if !ok {
		return fmt.Errorf("unknown report %q", kind)
	}
	var f Filters
	if err := json.Unmarshal(raw, &f); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, jobTimeout)
	defer cancel()
	asOf := time.Now()
	content, count, truncated, err := generate(ctx, conn, d, format, f, asOf)
	if err != nil {
		return err
	}
	return db.InTx(ctx, conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE report_jobs SET status = 'completed', content = $2, content_type = $3, size_bytes = $4, row_count = $5, truncated = $6,
			data_as_of = $7, finished_at = now() WHERE id = $1::uuid`, id, content, formats[format].contentType, len(content), count, truncated, asOf); err != nil {
			return err
		}
		return notifications.Enqueue(ctx, tx, "", notifications.Event{Type: notifications.ReportReady, ObjectType: "report", ObjectID: id, DedupeKey: "report-ready:" + id,
			Title: d.Title + " is ready", Body: fmt.Sprintf("%d rows.", count), Link: "/reports", Recipients: []string{user}})
	})
}
