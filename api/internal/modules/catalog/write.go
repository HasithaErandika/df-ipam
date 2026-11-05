package catalog

import (
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// readBody binds the request and applies the entity's own check.
func readBody(c *gin.Context, e Entity) (map[string]any, bool) {
	var body map[string]any
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return nil, false
	}
	if e.Check != nil {
		if err := e.Check(body); err != nil {
			httpx.Fail(c, err)
			return nil, false
		}
	}
	return body, true
}

// insertColumns is what one INSERT sets.
type insertColumns struct {
	cols, marks []string
	args        []any
}

func (i *insertColumns) add(col, cast string, v any) {
	i.args = append(i.args, v)
	i.cols = append(i.cols, col)
	i.marks = append(i.marks, fmt.Sprintf("$%d::%s", len(i.args), cast))
}

func newRow(e Entity, body map[string]any, actor string) (insertColumns, error) {
	var row insertColumns
	for _, f := range e.Fields {
		raw, present := body[f.JSON]
		v, err := checkValue(f, raw)
		if err != nil {
			return row, err
		}
		if present && v != nil {
			row.add(f.Col, casts[f.Kind], v)
		}
	}
	if e.Stamped {
		row.add("created_by", "uuid", actor)
		row.add("updated_by", "uuid", actor)
	}
	if e.ActorCol != "" {
		row.add(e.ActorCol, "uuid", actor)
	}
	return row, nil
}

func (h *Handler) create(c *gin.Context, e Entity) {
	body, ok := readBody(c, e)
	if !ok {
		return
	}
	actor := auth.Current(c).UserID
	row, err := newRow(e, body, actor)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	var out json.RawMessage
	err = db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx, fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING id::text",
			e.Table, strings.Join(row.cols, ", "), strings.Join(row.marks, ", ")), row.args...).Scan(&id); err != nil {
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

// changedColumns builds the SET list of an UPDATE from the fields in the body.
func changedColumns(e Entity, body map[string]any, id, actor string) ([]string, []any, error) {
	var sets []string
	args := []any{id}
	for _, f := range e.Fields {
		raw, present := body[f.JSON]
		if !present {
			continue
		}
		v, err := checkValue(f, raw)
		if err != nil {
			return nil, nil, err
		}
		args = append(args, v)
		sets = append(sets, fmt.Sprintf("%s = $%d::%s", f.Col, len(args), casts[f.Kind]))
	}
	if len(sets) == 0 {
		return nil, nil, httpx.Validation("body", "Nothing to update.")
	}
	if e.Stamped {
		args = append(args, actor)
		sets = append(sets, fmt.Sprintf("updated_by = $%d::uuid", len(args)))
	}
	return sets, args, nil
}

func (h *Handler) update(c *gin.Context, e Entity) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	body, ok := readBody(c, e)
	if !ok {
		return
	}
	actor := auth.Current(c).UserID
	sets, args, err := changedColumns(e, body, id, actor)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	ctx := c.Request.Context()
	var out json.RawMessage
	err = db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
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
