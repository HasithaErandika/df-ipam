// Package auditlog serves the audit trail. It is read-only by design.
package auditlog

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Entry struct {
	ID            int64           `json:"id"`
	ActorEmail    *string         `json:"actorEmail"`
	ActorType     string          `json:"actorType"`
	Action        string          `json:"action"`
	ObjectType    string          `json:"objectType"`
	ObjectID      *string         `json:"objectId"`
	OldValues     json.RawMessage `json:"oldValues"`
	NewValues     json.RawMessage `json:"newValues"`
	OccurredAt    time.Time       `json:"occurredAt"`
	SourceIP      *string         `json:"sourceIp"`
	CorrelationID *string         `json:"correlationId"`
	Result        string          `json:"result"`
}

type Handler struct{ pool db.Conn }

func NewHandler(pool db.Conn) *Handler { return &Handler{pool} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/audit-logs", h.list, auth.AuditView)
	g.Handle(http.MethodGet, "/audit-logs/filters", h.filterValues, auth.AuditView)
}

// filterValues lists the actions and object types that exist, for the filter pickers.
func (h *Handler) filterValues(c *gin.Context) {
	ctx := c.Request.Context()
	values := func(sql string) ([]string, error) {
		rows, err := h.pool.Query(ctx, sql)
		if err != nil {
			return nil, err
		}
		out, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if out == nil {
			out = []string{}
		}
		return out, err
	}
	actions, err := values(`SELECT DISTINCT action FROM audit_logs ORDER BY action LIMIT 500`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	objects, err := values(`SELECT DISTINCT object_type FROM audit_logs ORDER BY object_type LIMIT 500`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, gin.H{"actions": actions, "objectTypes": objects})
}

// filters builds the WHERE clause from fixed fragments. Values are always parameters.
func filters(c *gin.Context) (string, []any, error) {
	var where []string
	var args []any
	add := func(fragment string, value any) {
		args = append(args, value)
		where = append(where, strings.ReplaceAll(fragment, "?", "$"+strconv.Itoa(len(args))))
	}
	if v := c.Query("objectType"); v != "" {
		add("a.object_type = ?", v)
	}
	if v := c.Query("action"); v != "" {
		add("a.action = ?", v)
	}
	if v := c.Query("result"); v != "" {
		add("a.result = ?", v)
	}
	if v := strings.TrimSpace(c.Query("actor")); v != "" {
		add("u.email ILIKE '%' || ? || '%'", escapeLike(v))
	}
	for _, f := range []struct{ name, fragment string }{{"from", "a.occurred_at >= ?"}, {"to", "a.occurred_at < ?"}} {
		v := c.Query(f.name)
		if v == "" {
			continue
		}
		day, err := time.Parse("2006-01-02", v)
		if err != nil {
			return "", nil, httpx.Validation(f.name, "Use a date in the form YYYY-MM-DD.")
		}
		if f.name == "to" {
			day = day.AddDate(0, 0, 1) // the end date includes that whole day
		}
		add(f.fragment, day)
	}
	if len(where) == 0 {
		return "", nil, nil
	}
	return " WHERE " + strings.Join(where, " AND "), args, nil
}

func escapeLike(v string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(v)
}

func (h *Handler) list(c *gin.Context) {
	p := httpx.ParsePage(c)
	ctx := c.Request.Context()
	clause, args, err := filters(c)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	const from = ` FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id`
	var total int64
	if err := h.pool.QueryRow(ctx, `SELECT count(*)`+from+clause, args...).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	args = append(args, p.Size, p.Offset())
	rows, err := h.pool.Query(ctx, `
		SELECT a.id, u.email, a.actor_type, a.action, a.object_type, a.object_id, a.old_values, a.new_values,
		       a.occurred_at, host(a.source_ip), a.correlation_id, a.result`+from+clause+
		fmt.Sprintf(` ORDER BY a.occurred_at DESC, a.id DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Entry, error) {
		var e Entry
		return e, r.Scan(&e.ID, &e.ActorEmail, &e.ActorType, &e.Action, &e.ObjectType, &e.ObjectID, &e.OldValues,
			&e.NewValues, &e.OccurredAt, &e.SourceIP, &e.CorrelationID, &e.Result)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}
