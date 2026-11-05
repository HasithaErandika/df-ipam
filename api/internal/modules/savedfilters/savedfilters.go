// Package savedfilters stores each user's named filter sets for list screens.
// A user only ever sees and deletes their own.
package savedfilters

import (
	"encoding/json"
	"net/http"
	"strings"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// Pages that support saved filters.
var pages = map[string]bool{"subnets": true, "ip-addresses": true}

const maxKeys = 30

type Filter struct {
	ID      string            `json:"id"`
	Page    string            `json:"page"`
	Name    string            `json:"name"`
	Filters map[string]string `json:"filters"`
}

type Handler struct{ conn db.Conn }

func NewHandler(conn db.Conn) *Handler { return &Handler{conn} }

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/saved-filters", h.list, auth.InventoryView)
	g.Handle(http.MethodPost, "/saved-filters", h.create, auth.InventoryView)
	g.Handle(http.MethodDelete, "/saved-filters/:id", h.remove, auth.InventoryView)
}

func scan(r pgx.CollectableRow) (Filter, error) {
	var f Filter
	var raw []byte
	if err := r.Scan(&f.ID, &f.Page, &f.Name, &raw); err != nil {
		return f, err
	}
	return f, json.Unmarshal(raw, &f.Filters)
}

func (h *Handler) list(c *gin.Context) {
	p := httpx.ParsePage(c)
	page := c.Query("page")
	if !pages[page] {
		httpx.Fail(c, httpx.Validation("page", "Choose a page that supports saved filters."))
		return
	}
	ctx, user := c.Request.Context(), auth.Current(c).UserID
	var total int64
	if err := h.conn.QueryRow(ctx, `SELECT count(*) FROM saved_filters WHERE user_id = $1::uuid AND page = $2`, user, page).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := h.conn.Query(ctx, `SELECT id::text, page, name, filters FROM saved_filters WHERE user_id = $1::uuid AND page = $2
		ORDER BY name, id LIMIT $3 OFFSET $4`, user, page, p.Size, p.Offset())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, scan)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}

// Validate checks a filter set before it is stored.
func Validate(f *Filter) error {
	f.Name = strings.TrimSpace(f.Name)
	if !pages[f.Page] {
		return httpx.Validation("page", "Choose a page that supports saved filters.")
	}
	if f.Name == "" || len(f.Name) > 80 {
		return httpx.Validation("name", "Enter a name of up to 80 characters.")
	}
	if len(f.Filters) == 0 || len(f.Filters) > maxKeys {
		return httpx.Validation("filters", "Set at least one filter before saving.")
	}
	for k, v := range f.Filters {
		if k == "" || len(k) > 40 || len(v) > 200 || k == "page" || k == "pageSize" {
			return httpx.Validation("filters", "A filter value is not valid.")
		}
	}
	return nil
}

func (h *Handler) create(c *gin.Context) {
	var f Filter
	if err := c.ShouldBindJSON(&f); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	if err := Validate(&f); err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx, user := c.Request.Context(), auth.Current(c).UserID
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		raw, _ := json.Marshal(f.Filters)
		if err := tx.QueryRow(ctx, `INSERT INTO saved_filters (user_id, page, name, filters) VALUES ($1::uuid, $2, $3, $4::jsonb) RETURNING id::text`,
			user, f.Page, f.Name, raw).Scan(&f.ID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: user, Action: "saved_filter.create", ObjectType: "saved_filter", ObjectID: f.ID, New: f})
	})
	if db.ErrorCode(err) == db.CodeUnique {
		httpx.Fail(c, httpx.NewError(http.StatusConflict, "ALREADY_EXISTS", "You already have a saved filter with this name."))
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, f)
}

func (h *Handler) remove(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ctx, user := c.Request.Context(), auth.Current(c).UserID
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `DELETE FROM saved_filters WHERE id = $1::uuid AND user_id = $2::uuid RETURNING id::text, page, name, filters`, id, user)
		if err != nil {
			return err
		}
		old, err := pgx.CollectExactlyOneRow(rows, scan)
		if err == pgx.ErrNoRows {
			return httpx.ErrNotFound // someone else's filter is simply not found
		}
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: user, Action: "saved_filter.delete", ObjectType: "saved_filter", ObjectID: id, Old: old})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
