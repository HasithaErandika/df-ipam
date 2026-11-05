// Package organisations manages organisations. Creating one also creates its
// Default routing domain, so subnets always have a routing context.
package organisations

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"dfipam/api/internal/audit"
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
	g.Handle(http.MethodGet, "/organisations/:id", h.get, auth.InventoryView)
	g.Handle(http.MethodPatch, "/organisations/:id", h.update, auth.InventoryEdit)
	g.Handle(http.MethodDelete, "/organisations/:id", h.remove, auth.InventoryDelete)
}

func load(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id string, lock bool) (Organisation, error) {
	sql := `SELECT id::text, name, code, created_at FROM organisations WHERE id = $1::uuid`
	if lock {
		sql += " FOR UPDATE"
	}
	var o Organisation
	err := q.QueryRow(ctx, sql, id).Scan(&o.ID, &o.Name, &o.Code, &o.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return o, httpx.ErrNotFound
	}
	return o, err
}

func (h *Handler) get(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	o, err := load(c.Request.Context(), h.pool, id, false)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, o)
}

func (h *Handler) update(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req struct {
		Name *string `json:"name"`
		Code *string `json:"code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	var out Organisation
	err := db.InTx(ctx, h.pool, func(tx pgx.Tx) error {
		old, err := load(ctx, tx, id, true)
		if err != nil {
			return err
		}
		next := old
		if req.Name != nil {
			if next.Name = strings.TrimSpace(*req.Name); next.Name == "" {
				return httpx.Validation("name", "Enter a name.")
			}
		}
		if req.Code != nil {
			if next.Code = strings.ToUpper(strings.TrimSpace(*req.Code)); next.Code == "" {
				return httpx.Validation("code", "Enter a short code.")
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE organisations SET name = $2, code = $3, updated_by = $4::uuid WHERE id = $1::uuid`,
			id, next.Name, next.Code, actor); err != nil {
			return err
		}
		out = next
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "organisation.update", ObjectType: "organisation", ObjectID: id, Old: old, New: next})
	})
	if db.ErrorCode(err) == db.CodeUnique {
		httpx.Fail(c, httpx.NewError(http.StatusConflict, "ALREADY_EXISTS", "An organisation with this name or code already exists."))
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

// remove deletes an organisation and its routing domains, but only when nothing
// else refers to them. Anything in use is refused by the foreign keys.
func (h *Handler) remove(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	err := db.InTx(ctx, h.pool, func(tx pgx.Tx) error {
		old, err := load(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM routing_domains WHERE organisation_id = $1::uuid`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM organisations WHERE id = $1::uuid`, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "organisation.delete", ObjectType: "organisation", ObjectID: id, Old: old})
	})
	if db.ErrorCode(err) == db.CodeForeignKey {
		httpx.Fail(c, httpx.NewError(http.StatusConflict, "RELATED_RECORD", "This organisation still has sites, routing domains in use, or other linked records."))
		return
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
		if err := tx.QueryRow(ctx, `INSERT INTO organisations (name, code, created_by, updated_by)
			VALUES ($1, $2, $3::uuid, $3::uuid) RETURNING id::text, name, code, created_at`, req.Name, req.Code, actor).
			Scan(&o.ID, &o.Name, &o.Code, &o.CreatedAt); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO routing_domains (organisation_id, name, kind, created_by, updated_by)
			VALUES ($1::uuid, 'Default', 'default', $2::uuid, $2::uuid)`, o.ID, actor); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "organisation.create", ObjectType: "organisation", ObjectID: o.ID, New: o})
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
