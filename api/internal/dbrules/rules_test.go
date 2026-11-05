// Package dbrules tests the rules enforced by the schema itself. It needs a
// migrated database: set TEST_DATABASE_URL, otherwise the tests are skipped.
// Every test runs in a transaction that is rolled back, so nothing is kept.
package dbrules

import (
	"context"
	"os"
	"testing"

	"dfipam/api/internal/platform/db"

	"github.com/jackc/pgx/v5"
)

type env struct {
	t   *testing.T
	tx  pgx.Tx
	rd  string
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
	e := env{t: t, tx: tx, ctx: ctx}
	var org string
	e.row(&org, `INSERT INTO organisations (name, code) VALUES ('Rules Test', 'RULES-TEST') RETURNING id`)
	e.row(&e.rd, `INSERT INTO routing_domains (organisation_id, name) VALUES ($1, 'A') RETURNING id`, org)
	return e
}

func (e env) row(dest any, sql string, args ...any) {
	e.t.Helper()
	if err := e.tx.QueryRow(e.ctx, sql, args...).Scan(dest); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
}

// try runs sql in a savepoint, so an expected failure leaves the test usable.
func (e env) try(sql string, args ...any) string {
	e.t.Helper()
	sp, err := e.tx.Begin(e.ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	defer sp.Rollback(e.ctx)
	if _, err := sp.Exec(e.ctx, sql, args...); err != nil {
		code := db.ErrorCode(err)
		if code == "" {
			e.t.Fatal(err)
		}
		return code
	}
	if err := sp.Commit(e.ctx); err != nil {
		e.t.Fatal(err)
	}
	return ""
}

func (e env) subnet(cidr string) string {
	return e.try(`INSERT INTO subnets (routing_domain_id, cidr, name, environment) VALUES ($1, $2::text::cidr, 's', 'testing')`, e.rd, cidr)
}

func (e env) exception() string {
	var user, id string
	e.row(&user, `INSERT INTO users (email, auth_provider, auth_subject) VALUES ('r@example.com', 'test', gen_random_uuid()::text) RETURNING id`)
	e.row(&id, `INSERT INTO overlap_exceptions (reason, approved_by) VALUES ('test', $1) RETURNING id`, user)
	return id
}

func TestOverlapBlockedInSameDomain(t *testing.T) {
	e := setup(t)
	if code := e.subnet("10.0.0.0/24"); code != "" {
		t.Fatal(code)
	}
	if code := e.subnet("10.0.0.128/25"); code != db.CodeExclusion {
		t.Fatalf("got %q", code)
	}
}

func TestExceptionSubnetStaysProtected(t *testing.T) {
	e := setup(t)
	ex := e.exception()
	if code := e.try(`INSERT INTO subnets (routing_domain_id, cidr, name, environment, overlap_exception_id)
		VALUES ($1, '10.9.0.0/24', 'x', 'testing', $2)`, e.rd, ex); code != "" {
		t.Fatal(code)
	}
	if code := e.subnet("10.9.0.0/25"); code != db.CodeExclusion {
		t.Fatalf("plain subnet overlapped an exception subnet: %q", code)
	}
}

func TestApprovedOverlapDoesNotBlockRename(t *testing.T) {
	e := setup(t)
	ex := e.exception()
	e.subnet("10.8.0.0/24")
	if code := e.try(`INSERT INTO subnets (routing_domain_id, cidr, name, environment, overlap_exception_id)
		VALUES ($1, '10.8.0.0/25', 'x', 'testing', $2)`, e.rd, ex); code != "" {
		t.Fatal(code)
	}
	if code := e.try(`UPDATE subnets SET name = 'renamed' WHERE routing_domain_id = $1 AND cidr = '10.8.0.0/24'`, e.rd); code != "" {
		t.Fatalf("rename blocked: %s", code)
	}
}

func TestAddressMustBeInsideSubnet(t *testing.T) {
	e := setup(t)
	var id string
	e.row(&id, `INSERT INTO subnets (routing_domain_id, cidr, name, environment) VALUES ($1, '10.7.0.0/24', 's', 'testing') RETURNING id`, e.rd)
	if code := e.try(`INSERT INTO ip_addresses (routing_domain_id, subnet_id, address) VALUES ($1, $2, '10.6.0.1')`, e.rd, id); code != db.CodeCheck {
		t.Fatalf("got %q", code)
	}
}

func TestAuditLogsAreAppendOnly(t *testing.T) {
	e := setup(t)
	var id int64
	e.row(&id, `INSERT INTO audit_logs (action, object_type) VALUES ('test', 'test') RETURNING id`)
	if code := e.try(`UPDATE audit_logs SET action = 'changed' WHERE id = $1`, id); code != "42501" {
		t.Fatalf("update: %q", code)
	}
	if code := e.try(`DELETE FROM audit_logs WHERE id = $1`, id); code != "42501" {
		t.Fatalf("delete: %q", code)
	}
}

func TestSelfApprovalBlocked(t *testing.T) {
	e := setup(t)
	var user, subnet, request string
	e.row(&user, `INSERT INTO users (email, auth_provider, auth_subject) VALUES ('s@example.com', 'test', gen_random_uuid()::text) RETURNING id`)
	e.row(&subnet, `INSERT INTO subnets (routing_domain_id, cidr, name, environment) VALUES ($1, '10.5.0.0/24', 's', 'testing') RETURNING id`, e.rd)
	e.row(&request, `INSERT INTO ip_requests (subnet_id, requested_by) VALUES ($1, $2) RETURNING id`, subnet, user)
	if code := e.try(`INSERT INTO ip_request_approvals (request_id, step_order, approver_id) VALUES ($1, 1, $2)`, request, user); code != db.CodeCheck {
		t.Fatalf("got %q", code)
	}
}

func TestDiscoverySnapshotIsFrozen(t *testing.T) {
	e := setup(t)
	var id string
	e.row(&id, `INSERT INTO discovery_jobs (config_snapshot) VALUES ('{"a":1}') RETURNING id`)
	if code := e.try(`UPDATE discovery_jobs SET config_snapshot = '{"a":2}' WHERE id = $1`, id); code != db.CodeCheck {
		t.Fatalf("got %q", code)
	}
	if code := e.try(`UPDATE discovery_jobs SET status = 'running' WHERE id = $1`, id); code != "" {
		t.Fatalf("status update blocked: %s", code)
	}
}

// Every table must stay closed to the Supabase Data API.
func TestTablesAreClosedToDataAPI(t *testing.T) {
	e := setup(t)
	rows, err := e.tx.Query(e.ctx, `SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'r' AND NOT c.relrowsecurity`)
	if err != nil {
		t.Fatal(err)
	}
	open, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	if len(open) > 0 {
		t.Fatalf("row level security is off for: %v", open)
	}
	var grants int
	e.row(&grants, `SELECT count(*) FROM information_schema.role_table_grants
		WHERE table_schema = 'public' AND grantee IN ('anon', 'authenticated')`)
	if grants != 0 {
		t.Fatalf("%d table grants remain for anon or authenticated", grants)
	}
}
