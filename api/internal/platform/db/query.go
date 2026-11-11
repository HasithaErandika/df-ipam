package db

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

// Where builds a parameterised WHERE clause. Conditions are constants from the
// caller; only values come from a request, and they are always bound.
type Where struct {
	parts []string
	Args  []any
}

// Add appends a condition. Each ? becomes the next placeholder, bound to value.
func (w *Where) Add(cond string, value any) {
	w.Args = append(w.Args, value)
	w.parts = append(w.parts, strings.ReplaceAll(cond, "?", fmt.Sprintf("$%d", len(w.Args))))
}

// Raw appends a constant condition that binds no value.
func (w *Where) Raw(cond string) { w.parts = append(w.parts, cond) }

// AddIf adds the condition only when ok is true.
func (w *Where) AddIf(ok bool, cond string, value any) {
	if ok {
		w.Add(cond, value)
	}
}

// SQL is the clause with a leading space, or empty when there are no conditions.
func (w *Where) SQL() string {
	if len(w.parts) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(w.parts, " AND ")
}

// ListPage runs one page of a list and its total. query must select columns named
// like T's db tags and end with ORDER BY; LIMIT and OFFSET are appended here.
func ListPage[T any](ctx context.Context, conn Conn, query, count string, args []any, p httpx.Page) ([]T, int64, error) {
	var total int64
	if err := conn.QueryRow(ctx, count, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	n := len(args)
	paged := append(append([]any{}, args...), p.Size, p.Offset())
	rows, err := conn.Query(ctx, fmt.Sprintf("%s LIMIT $%d OFFSET $%d", query, n+1, n+2), paged...)
	if err != nil {
		return nil, 0, err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[T])
	return items, total, err
}

// All runs a query and collects every row into T by db tag.
func All[T any](ctx context.Context, conn Conn, query string, args ...any) ([]T, error) {
	rows, err := conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[T])
}

// Friendly turns database failures into errors that are safe to show. A message
// given for conflicts replaces the generic one.
func Friendly(err error, conflict ...string) error {
	if err == nil {
		return nil
	}
	var he *httpx.Error
	if errors.As(err, &he) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.ErrNotFound
	}
	switch ErrorCode(err) {
	case CodeUnique, CodeExclusion:
		msg := "That value is already in use."
		if len(conflict) > 0 {
			msg = conflict[0]
		}
		return httpx.NewError(http.StatusConflict, "CONFLICT", msg)
	case CodeForeignKey:
		return httpx.NewError(http.StatusConflict, "IN_USE", "This record is still referenced by other records.")
	case CodeCheck, CodeBadValue:
		return httpx.NewError(http.StatusUnprocessableEntity, "VALIDATION_FAILED", "The values do not fit the selected segment or allowed format.")
	}
	return err
}

// Contains turns search text into an ILIKE pattern, escaping wildcards so a
// person's % or _ matches literally. Use with ILIKE ... ESCAPE '\'.
func Contains(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(strings.TrimSpace(q)) + "%"
}
