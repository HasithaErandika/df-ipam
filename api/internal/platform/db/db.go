// Package db holds the connection pool and transaction helper.
package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
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
