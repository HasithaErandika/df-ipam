// Package dbrules tests the rules the schema enforces itself. It needs a migrated
// database: set TEST_DATABASE_URL, otherwise the tests are skipped. Every test runs
// in a transaction that is rolled back, so nothing is kept.
package dbrules

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
)

type env struct {
	t   *testing.T
	tx  pgx.Tx
	ctx context.Context
}

func setup(t *testing.T) env {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(ctx) })
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tx.Rollback(ctx) })
	return env{t: t, tx: tx, ctx: ctx}
}

func (e env) row(dest any, sql string, args ...any) {
	e.t.Helper()
	if err := e.tx.QueryRow(e.ctx, sql, args...).Scan(dest); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
}

// try runs sql in a savepoint and returns the error text, or "" on success.
func (e env) try(sql string, args ...any) string {
	e.t.Helper()
	sp, err := e.tx.Begin(e.ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err = sp.Exec(e.ctx, sql, args...); err != nil {
		_ = sp.Rollback(e.ctx)
		return err.Error()
	}
	if err := sp.Commit(e.ctx); err != nil {
		e.t.Fatal(err)
	}
	return ""
}

func (e env) mustFail(what, sql string, args ...any) {
	e.t.Helper()
	if e.try(sql, args...) == "" {
		e.t.Errorf("%s: was accepted", what)
	}
}

func (e env) mustPass(what, sql string, args ...any) {
	e.t.Helper()
	if msg := e.try(sql, args...); msg != "" {
		e.t.Errorf("%s: %s", what, msg)
	}
}

// segment adds a segment and returns its id.
func (e env) segment(name, kind, network string) string {
	e.t.Helper()
	var id string
	e.row(&id, `INSERT INTO segments (name, kind, network) VALUES ($1, $2, $3::cidr) RETURNING id`, name, kind, network)
	return id
}
