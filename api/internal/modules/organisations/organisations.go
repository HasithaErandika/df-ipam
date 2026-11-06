// Package organisations manages organisations. Creating one also creates its
// Default routing domain, so subnets always have a routing context.
package organisations

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Organisation struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Code      string    `json:"code"`
	CreatedAt time.Time `json:"createdAt"`
}

type Handler struct{ pool db.Conn }

func NewHandler(pool db.Conn) *Handler { return &Handler{pool} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/organisations", h.list, auth.InventoryView)
	g.Handle(http.MethodPost, "/organisations", h.create, auth.InventoryCreate)
	g.Handle(http.MethodPatch, "/organisations/:id", h.update, auth.InventoryEdit)
	g.Handle(http.MethodDelete, "/organisations/:id", h.remove, auth.InventoryDelete)
}

func (h *Handler) one(c *gin.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id string, lock bool) (Organisation, error) {
	sql := `SELECT id::text, name, code, created_at FROM organisations WHERE id = $1::uuid`
	if lock {
		sql += " FOR UPDATE"
	}
	var o Organisation
	err := q.QueryRow(c.Request.Context(), sql, id).Scan(&o.ID, &o.Name, &o.Code, &o.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return o, httpx.ErrNotFound
	}
	return o, err
}

// remove deletes an organisation with nothing in it. Its empty Default routing
// domain goes with it; anything else must be removed first.
func (h *Handler) remove(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	err := db.InTx(ctx, h.pool, func(tx pgx.Tx) error {
		old, err := h.one(c, tx, id, true)
		if err != nil {
			return err
		}
		return Remove(ctx, tx, actor, id, old)
	})
	if db.ErrorCode(err) == db.CodeForeignKey {
		err = httpx.NewError(http.StatusConflict, "ORGANISATION_IN_USE", "This organisation still has routing domains, sites or other records. Remove them first.")
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) list(c *gin.Context) {
	p := httpx.ParsePage(c)
	ctx := c.Request.Context()
	var total int64
	if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM organisations`).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := h.pool.Query(ctx, `SELECT id::text, name, code, created_at FROM organisations ORDER BY name, id LIMIT $1 OFFSET $2`, p.Size, p.Offset())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Organisation, error) {
		var o Organisation
		return o, r.Scan(&o.ID, &o.Name, &o.Code, &o.CreatedAt)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}

func (h *Handler) create(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	req.Name, req.Code = strings.TrimSpace(req.Name), strings.ToUpper(strings.TrimSpace(req.Code))
	if req.Name == "" {
		httpx.Fail(c, httpx.Validation("name", "Enter a name."))
		return
	}
	if req.Code == "" {
		httpx.Fail(c, httpx.Validation("code", "Enter a short code."))
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var o Organisation
	err := db.InTx(ctx, h.pool, func(tx pgx.Tx) error {
		var err error
		o, err = Insert(ctx, tx, actor, req.Name, req.Code)
		return err
	})
	if db.ErrorCode(err) == db.CodeUnique {
		httpx.Fail(c, httpx.NewError(http.StatusConflict, "ALREADY_EXISTS", "An organisation with this name or code already exists."))
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, o)
}
