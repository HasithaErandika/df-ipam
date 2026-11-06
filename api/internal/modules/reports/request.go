package reports

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
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

type reportRequest struct {
	ReportType string  `json:"reportType"`
	Format     string  `json:"format"`
	Filters    Filters `json:"filters"`
}

// check validates the request for this person, and says whether they see everything.
func (r *reportRequest) check(p *auth.Principal) (full bool, err error) {
	d, known := definition(r.ReportType)
	if !known {
		return false, httpx.Validation("reportType", "Choose a valid report.")
	}
	if _, ok := formats[r.Format]; !ok {
		return false, httpx.Validation("format", "Choose Excel, CSV or PDF.")
	}
	r.Filters.Scope = nil
	if err := r.Filters.validate(d); err != nil {
		return false, err
	}
	full, ok := allowed(p, d)
	if !ok {
		return false, httpx.ErrForbidden
	}
	return full, nil
}

func (h *Handler) create(c *gin.Context) {
	var req reportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	principal := auth.Current(c)
	full, err := req.check(principal)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx, actor := c.Request.Context(), principal.UserID
	var out Job
	err = db.InTx(ctx, h.conn, func(tx pgx.Tx) (err error) {
		out, err = queueReport(ctx, tx, actor, full, req)
		return err
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusAccepted, out)
}

func queueReport(ctx context.Context, tx pgx.Tx, actor string, full bool, req reportRequest) (Job, error) {
	// Serialises one person's requests, so the limit can't be raced past.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 7))`, actor); err != nil {
		return Job{}, err
	}
	var active int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM report_jobs WHERE requested_by = $1::uuid AND status IN ('queued', 'running')`, actor).Scan(&active); err != nil {
		return Job{}, err
	}
	if active >= maxActivePerUser {
		return Job{}, errBusy
	}
	if !full {
		scope, err := scopeOf(ctx, tx, actor)
		if err != nil {
			return Job{}, err
		}
		req.Filters.Scope = &scope
	}
	raw, err := json.Marshal(req.Filters)
	if err != nil {
		return Job{}, err
	}
	out, err := scanJob(tx.QueryRow(ctx, `INSERT INTO report_jobs (report_type, format, filters, requested_by) VALUES ($1, $2, $3::jsonb, $4::uuid) RETURNING `+jobColumns,
		req.ReportType, req.Format, raw, actor))
	if err != nil {
		return out, err
	}
	return out, audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "report.request", ObjectType: "report", ObjectID: out.ID, New: req})
}
