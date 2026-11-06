package allocation

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// Repository holds the workflow's SQL. It reads subnets and addresses directly,
// because allocation must lock them in its own transaction.
type Repository struct{}

type where struct {
	clauses []string
	args    []any
}

func (w *where) add(clause string, v any) {
	w.args = append(w.args, v)
	w.clauses = append(w.clauses, fmt.Sprintf(clause, len(w.args)))
}

func (w *where) sql() string {
	if len(w.clauses) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(w.clauses, " AND ")
}
