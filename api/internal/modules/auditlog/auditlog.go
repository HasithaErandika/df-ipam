// Package auditlog serves the audit trail. It is read-only by design.
package auditlog

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
}

func (h *Handler) list(c *gin.Context) {
	p := httpx.ParsePage(c)
	ctx := c.Request.Context()
	objectType := c.Query("objectType")
	var total int64
	if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE $1 = '' OR object_type = $1`, objectType).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := h.pool.Query(ctx, `
		SELECT a.id, u.email, a.actor_type, a.action, a.object_type, a.object_id, a.old_values, a.new_values,
		       a.occurred_at, host(a.source_ip), a.correlation_id, a.result
		FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id
		WHERE $1 = '' OR a.object_type = $1
		ORDER BY a.occurred_at DESC, a.id DESC LIMIT $2 OFFSET $3`, objectType, p.Size, p.Offset())
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
