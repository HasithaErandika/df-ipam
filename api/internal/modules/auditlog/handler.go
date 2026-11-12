// Package auditlog lets people read and export the append-only change history.
package auditlog

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/platform/tabular"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
)

// maxExport bounds one CSV export. Narrow the dates to get the rest.
const maxExport = 50_000

type Handler struct{ conn db.Conn }

func New(conn db.Conn) *Handler { return &Handler{conn: conn} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/audit-logs", h.list, auth.AuditView)
	g.Handle(http.MethodGet, "/audit-logs/export", h.export, auth.AuditView)
}

type entryDTO struct {
	ID            int64           `db:"id" json:"id"`
	OccurredAt    time.Time       `db:"occurred_at" json:"occurredAt"`
	ActorEmail    *string         `db:"actor_email" json:"actorEmail"`
	ActorType     string          `db:"actor_type" json:"actorType"`
	Action        string          `db:"action" json:"action"`
	ObjectType    string          `db:"object_type" json:"objectType"`
	ObjectID      *string         `db:"object_id" json:"objectId"`
	OldValues     json.RawMessage `db:"old_values" json:"oldValues"`
	NewValues     json.RawMessage `db:"new_values" json:"newValues"`
	SourceIP      *string         `db:"source_ip" json:"sourceIp"`
	CorrelationID *string         `db:"correlation_id" json:"correlationId"`
}

const entryFrom = ` FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id`

const entryColumns = `SELECT a.id, a.occurred_at, u.email AS actor_email, a.actor_type, a.action, a.object_type, a.object_id,
	a.old_values, a.new_values, host(a.source_ip) AS source_ip, a.correlation_id`

func filter(c *gin.Context) (db.Where, error) {
	var w db.Where
	var v validate.V
	if s := strings.TrimSpace(c.Query("user")); s != "" {
		w.Add(`u.email ILIKE ? ESCAPE '\'`, db.Contains(s))
	}
	if s := c.Query("action"); s != "" {
		v.Text("action", &s, 40, false, "Action")
		w.Add("a.action = ?", s)
	}
	if s := c.Query("objectType"); s != "" {
		v.Text("objectType", &s, 40, false, "Record type")
		w.Add("a.object_type = ?", s)
	}
	from, to := c.Query("from"), c.Query("to")
	v.Date("from", &from, false, "From")
	v.Date("to", &to, false, "To")
	w.AddIf(from != "", "a.occurred_at >= ?::date", from)
	w.AddIf(to != "", "a.occurred_at < ?::date + 1", to)
	if from != "" && to != "" && to < from {
		v.Add("to", "The end date is before the start date.")
	}
	// Free text matches the record ID or anything in the values, such as an IP or MAC.
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		w.Add(`(a.object_id ILIKE ? ESCAPE '\' OR a.old_values::text ILIKE ? ESCAPE '\' OR a.new_values::text ILIKE ? ESCAPE '\')`, db.Contains(q))
	}
	return w, v.Err()
}

func (h *Handler) list(c *gin.Context) {
	w, err := filter(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	page := httpx.ParsePage(c)
	items, total, err := db.ListPage[entryDTO](c, h.conn, entryColumns+entryFrom+w.SQL()+" ORDER BY a.occurred_at DESC, a.id DESC",
		"SELECT count(*)"+entryFrom+w.SQL(), w.Args, page)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, items, page, total)
}

func (h *Handler) export(c *gin.Context) {
	format := c.DefaultQuery("format", "csv")
	if _, ok := tabular.Formats[format]; !ok {
		httpx.Fail(c, httpx.Validation("format", "Choose CSV or Excel."))
		return
	}
	w, err := filter(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := tabular.NewWriter(format, "Audit log", []string{"occurredAt", "actor", "action", "objectType", "objectId", "oldValues", "newValues", "sourceIp"})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := h.conn.Query(c, `SELECT to_char(a.occurred_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), COALESCE(u.email, a.actor_type), a.action,
		a.object_type, COALESCE(a.object_id, ''), COALESCE(a.old_values::text, ''), COALESCE(a.new_values::text, ''), COALESCE(host(a.source_ip), '')`+
		entryFrom+w.SQL()+" ORDER BY a.occurred_at DESC, a.id DESC LIMIT "+strconv.Itoa(maxExport), w.Args...)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		r := make([]string, 8)
		if err := rows.Scan(&r[0], &r[1], &r[2], &r[3], &r[4], &r[5], &r[6], &r[7]); err != nil {
			httpx.Fail(c, err)
			return
		}
		_ = out.Write(r)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(c, err)
		return
	}
	data, err := out.Bytes()
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="audit-log.`+format+`"`)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, tabular.Formats[format], data)
}
