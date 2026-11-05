// Package catalog serves the reference data that subnets and addresses map to.
// These tables only need validated create, read, update and delete, so one
// handler driven by allowlisted definitions replaces a dozen near-identical modules.
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Entity struct {
	Path, Table, Object string
	Fields              []Field
	Labels              []Label
	Filters             map[string]string // query parameter to uuid column
	Search              string            // column for ?q=
	Order               string
	Stamped             bool   // has created_by and updated_by
	ActorCol            string // set to the acting user on create
	WritePermission     string
	// ReadOnly entities come from read-only sources: DF-IPAM lists them but never writes them.
	ReadOnly bool
	// Check validates fields that need more than a type check, such as a schedule.
	Check func(body map[string]any) error
}

func (e Entity) object() string {
	parts := []string{"'id', t.id"}
	for _, f := range e.Fields {
		parts = append(parts, fmt.Sprintf("'%s', t.%s", f.JSON, f.Col))
	}
	for _, l := range e.Labels {
		parts = append(parts, fmt.Sprintf("'%s', %s", l.JSON, l.SQL))
	}
	return "jsonb_build_object(" + strings.Join(parts, ", ") + ")"
}

func translate(err error) error {
	switch db.ErrorCode(err) {
	case db.CodeUnique:
		return httpx.NewError(http.StatusConflict, "ALREADY_EXISTS", "A matching record already exists.")
	case db.CodeForeignKey:
		return httpx.NewError(http.StatusConflict, "RELATED_RECORD", "A related record is missing, or this record is still in use.")
	case db.CodeCheck, db.CodeBadValue:
		return httpx.NewError(http.StatusUnprocessableEntity, "VALIDATION_FAILED", "A value is outside what this record allows.")
	}
	return err
}

type Handler struct{ conn db.Conn }

func NewHandler(conn db.Conn) *Handler { return &Handler{conn} }

func (h *Handler) Register(g *auth.Guard) {
	for _, entity := range Entities {
		e := entity
		create, edit, remove := auth.InventoryCreate, auth.InventoryEdit, auth.InventoryDelete
		if e.WritePermission != "" {
			create, edit, remove = e.WritePermission, e.WritePermission, e.WritePermission
		}
		g.Handle(http.MethodGet, e.Path, func(c *gin.Context) { h.list(c, e) }, auth.InventoryView)
		g.Handle(http.MethodGet, e.Path+"/:id", func(c *gin.Context) { h.get(c, e) }, auth.InventoryView)
		if e.ReadOnly {
			continue
		}
		g.Handle(http.MethodPost, e.Path, func(c *gin.Context) { h.create(c, e) }, create)
		g.Handle(http.MethodPatch, e.Path+"/:id", func(c *gin.Context) { h.update(c, e) }, edit)
		g.Handle(http.MethodDelete, e.Path+"/:id", func(c *gin.Context) { h.remove(c, e) }, remove)
	}
}

func (h *Handler) list(c *gin.Context, e Entity) {
	p := httpx.ParsePage(c)
	var where []string
	var args []any
	for param, col := range e.Filters {
		if v := c.Query(param); v != "" {
			if !httpx.IsUUID(v) {
				httpx.Fail(c, httpx.Validation(param, "Must be a valid ID."))
				return
			}
			args = append(args, v)
			where = append(where, fmt.Sprintf("t.%s = $%d::uuid", col, len(args)))
		}
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" && e.Search != "" {
		args = append(args, q)
		where = append(where, fmt.Sprintf("t.%s::text ILIKE '%%' || $%d || '%%'", e.Search, len(args)))
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}
	ctx := c.Request.Context()
	var total int64
	if err := h.conn.QueryRow(ctx, "SELECT count(*) FROM "+e.Table+" t"+clause, args...).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	args = append(args, p.Size, p.Offset())
	rows, err := h.conn.Query(ctx, fmt.Sprintf("SELECT %s FROM %s t%s ORDER BY t.%s, t.id LIMIT $%d OFFSET $%d",
		e.object(), e.Table, clause, e.Order, len(args)-1, len(args)), args...)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[json.RawMessage])
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}

func load(c *gin.Context, q interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}, e Entity, id string) (json.RawMessage, error) {
	var out json.RawMessage
	err := q.QueryRow(c.Request.Context(), "SELECT "+e.object()+" FROM "+e.Table+" t WHERE t.id = $1::uuid", id).Scan(&out)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, httpx.ErrNotFound
	}
	return out, err
}

func (h *Handler) get(c *gin.Context, e Entity) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	out, err := load(c, h.conn, e, id)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

func (h *Handler) remove(c *gin.Context, e Entity) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		old, err := load(c, tx, e, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "DELETE FROM "+e.Table+" WHERE id = $1::uuid", id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: auth.Current(c).UserID, Action: e.Object + ".delete", ObjectType: e.Object, ObjectID: id, Old: old})
	})
	if err != nil {
		httpx.Fail(c, translate(err))
		return
	}
	c.Status(http.StatusNoContent)
}
