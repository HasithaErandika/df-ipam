// Package db holds the connection pool and transaction helper.
package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens a pool. DB_MAX_CONNS caps it, because Supabase's session pooler
// allows only 15 clients across the API and worker together.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.New("DATABASE_URL can't be read")
	}
	cfg.MaxConns = 5
	if n, err := strconv.Atoi(os.Getenv("DB_MAX_CONNS")); err == nil && n > 0 {
		cfg.MaxConns = int32(n)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Conn is satisfied by a pool and by a transaction, so a whole request can run
// inside a rolled-back transaction in tests.
type Conn interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Begin(context.Context) (pgx.Tx, error)
}

// InTx runs fn in a transaction, committing only if fn returns nil.
func InTx(ctx context.Context, c Conn, fn func(pgx.Tx) error) error {
	return pgx.BeginFunc(ctx, c, fn)
}

// ErrorCode returns the PostgreSQL error code, or "" for other errors.
func ErrorCode(err error) string {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code
	}
	return ""
}

const (
	CodeExclusion  = "23P01"
	CodeForeignKey = "23503"
	CodeUnique     = "23505"
	CodeCheck      = "23514"
	CodeBadValue   = "22P02"
)

// Field is one column a change request may set. Lists of fields are fixed in code,
// so a column name never comes from a request.
type Field struct {
	Name, Column string
	Cast         string // for example uuid or macaddr; empty for text
	Nullable     bool   // an empty value clears the column
}

// ApplyFields updates the named fields of one row in tx, and returns their values
// before and after for the audit row. table and extra are constants from the caller.
func ApplyFields(ctx context.Context, tx pgx.Tx, table, id, actor, extra string, fields []Field, changes map[string]string) (before, after map[string]string, err error) {
	names := make([]string, 0, len(changes))
	for k := range changes {
		names = append(names, k)
	}
	sort.Strings(names)
	var sets, reads []string
	var selected []Field
	args := []any{id, actor}
	for _, name := range names {
		for _, f := range fields {
			if f.Name != name {
				continue
			}
			args = append(args, changes[name])
			expr := fmt.Sprintf("$%d", len(args))
			if f.Nullable {
				expr = "NULLIF(" + expr + ", '')"
			}
			if f.Cast != "" {
				expr += "::" + f.Cast
			}
			sets = append(sets, f.Column+" = "+expr)
			reads = append(reads, "COALESCE("+f.Column+"::text, '')")
			selected = append(selected, f)
		}
	}
	if len(sets) == 0 {
		return nil, nil, errors.New("no fields to change")
	}
	read := func(lock string) (map[string]string, error) {
		values := make([]string, len(reads))
		dest := make([]any, len(reads))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := tx.QueryRow(ctx, "SELECT "+strings.Join(reads, ", ")+" FROM "+table+" WHERE id = $1::uuid"+lock, id).Scan(dest...); err != nil {
			return nil, err
		}
		out := make(map[string]string, len(values))
		for i, f := range selected {
			out[f.Name] = values[i]
		}
		return out, nil
	}
	if before, err = read(" FOR UPDATE"); err != nil {
		return nil, nil, err
	}
	if _, err = tx.Exec(ctx, "UPDATE "+table+" SET "+strings.Join(sets, ", ")+", updated_by = $2::uuid"+extra+" WHERE id = $1::uuid", args...); err != nil {
		return nil, nil, err
	}
	after, err = read("")
	return before, after, err
}
