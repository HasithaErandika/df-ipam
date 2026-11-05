// Package catalog serves the reference data that subnets and addresses map to.
// These tables only need validated create, read, update and delete, so one
// handler driven by allowlisted definitions replaces a dozen near-identical modules.
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Kind int

const (
	Text Kind = iota
	UUID
	Int
	Bool
	CIDR
	Inet
	MAC
)

var casts = map[Kind]string{Text: "text", UUID: "uuid", Int: "integer", Bool: "boolean", CIDR: "text::cidr", Inet: "text::inet", MAC: "text::macaddr"}

// Field maps a JSON property to a column. Names here are constants, never input.
type Field struct {
	JSON, Col string
	Kind      Kind
	Required  bool
}

// Label adds a read-only related value, such as a parent's name.
type Label struct{ JSON, SQL string }

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

func checkValue(f Field, v any) (any, error) {
	if v == nil || v == "" && f.Kind != Text {
		if f.Required {
			return nil, httpx.Validation(f.JSON, "This field is required.")
		}
		return nil, nil
	}
	invalid := func(issue string) (any, error) { return nil, httpx.Validation(f.JSON, issue) }
	switch f.Kind {
	case Int:
		n, ok := v.(float64)
		if !ok || n != float64(int64(n)) {
			return invalid("Enter a whole number.")
		}
		return int64(n), nil
	case Bool:
		b, ok := v.(bool)
		if !ok {
			return invalid("Enter true or false.")
		}
		return b, nil
	}
	s, ok := v.(string)
	if !ok {
		return invalid("Enter text.")
	}
	s = strings.TrimSpace(s)
	switch f.Kind {
	case Text:
		if f.Required && s == "" {
			return invalid("This field is required.")
		}
	case UUID:
		if !httpx.IsUUID(s) {
			return invalid("Choose a valid record.")
		}
	case CIDR:
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return invalid("Enter a valid CIDR.")
		}
		if p.Masked() != p {
			return invalid(fmt.Sprintf("Host bits are set. Did you mean %s?", p.Masked()))
		}
	case Inet:
		if _, err := netip.ParseAddr(s); err != nil {
			return invalid("Enter a valid IP address.")
		}
	case MAC:
		if _, err := net.ParseMAC(s); err != nil {
			return invalid("Enter a valid MAC address.")
		}
	}
	return s, nil
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

func (h *Handler) create(c *gin.Context, e Entity) {
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	var cols, marks []string
	var args []any
	add := func(col, cast string, v any) {
		args = append(args, v)
		cols = append(cols, col)
		marks = append(marks, fmt.Sprintf("$%d::%s", len(args), cast))
	}
	for _, f := range e.Fields {
		raw, present := body[f.JSON]
		v, err := checkValue(f, raw)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		if present && v != nil {
			add(f.Col, casts[f.Kind], v)
		}
	}
	actor := auth.Current(c).UserID
	if e.Stamped {
		add("created_by", "uuid", actor)
		add("updated_by", "uuid", actor)
	}
	if e.ActorCol != "" {
		add(e.ActorCol, "uuid", actor)
	}
	ctx := c.Request.Context()
	var out json.RawMessage
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx, fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING id::text",
			e.Table, strings.Join(cols, ", "), strings.Join(marks, ", ")), args...).Scan(&id); err != nil {
			return err
		}
		var err error
		if out, err = load(c, tx, e, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: e.Object + ".create", ObjectType: e.Object, ObjectID: id, New: out})
	})
	if err != nil {
		httpx.Fail(c, translate(err))
		return
	}
	httpx.OK(c, http.StatusCreated, out)
}

func (h *Handler) update(c *gin.Context, e Entity) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	var sets []string
	args := []any{id}
	for _, f := range e.Fields {
		raw, present := body[f.JSON]
		if !present {
			continue
		}
		v, err := checkValue(f, raw)
		if err != nil {
			httpx.Fail(c, err)
			return
		}
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s = $%d::%s", f.Col, len(args), casts[f.Kind]))
	}
	if len(sets) == 0 {
		httpx.Fail(c, httpx.Validation("body", "Nothing to update."))
		return
	}
	actor := auth.Current(c).UserID
	if e.Stamped {
		args = append(args, actor)
		sets = append(sets, fmt.Sprintf("updated_by = $%d::uuid", len(args)))
	}
	ctx := c.Request.Context()
	var out json.RawMessage
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		old, err := load(c, tx, e, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE "+e.Table+" SET "+strings.Join(sets, ", ")+" WHERE id = $1::uuid", args...); err != nil {
			return err
		}
		if out, err = load(c, tx, e, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: e.Object + ".update", ObjectType: e.Object, ObjectID: id, Old: old, New: out})
	})
	if err != nil {
		httpx.Fail(c, translate(err))
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
