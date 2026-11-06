package allocation

import (
	"context"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/httpx"
	"errors"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	t       *testing.T
	admin   *pgxpool.Pool
	svc     *Service
	users   []string
	org     string
	subnets []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	appURL := os.Getenv("TEST_APP_DATABASE_URL")
	if appURL == "" {
		appURL = url
	}
	cfg, err := pgxpool.ParseConfig(appURL)
	if err != nil {
		t.Fatal(err)
	}
	// Supabase's session pooler allows 15 clients, so make test-db asks for fewer.
	cfg.MaxConns = 25
	if n, err := strconv.Atoi(os.Getenv("TEST_DB_MAX_CONNS")); err == nil && n > 0 {
		cfg.MaxConns = int32(n)
	}
	app, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, admin: admin, svc: NewService(app)}
	t.Cleanup(func() {
		f.cleanup()
		app.Close()
		admin.Close()
	})
	code := fmt.Sprintf("conc-%s", t.Name())
	f.row(&f.org, `INSERT INTO organisations (name, code) VALUES ($1, $1) RETURNING id::text`, code)
	return f
}

func (f *fixture) row(dest any, sql string, args ...any) {
	f.t.Helper()
	if err := f.admin.QueryRow(context.Background(), sql, args...).Scan(dest); err != nil {
		f.t.Fatalf("%s: %v", sql, err)
	}
}

func (f *fixture) user(name string) *auth.Principal {
	var id string
	f.row(&id, `INSERT INTO users (email, auth_provider, auth_subject) VALUES ($1, 'concurrency-test', gen_random_uuid()::text) RETURNING id::text`,
		name+"@concurrency.test")
	f.users = append(f.users, id)
	return &auth.Principal{UserID: id, Email: name + "@concurrency.test",
		Permissions: map[string]bool{auth.IPAllocate: true, auth.IPReserve: true, auth.RequestsApprove: true}}
}

func (f *fixture) subnet(cidr string) string {
	var rd, id string
	f.row(&rd, `INSERT INTO routing_domains (organisation_id, name, kind) VALUES ($1::uuid, $2, 'vrf') RETURNING id::text`, f.org, "rd-"+cidr)
	f.row(&id, `INSERT INTO subnets (routing_domain_id, cidr, name, environment) VALUES ($1::uuid, $2::cidr, 's', 'testing') RETURNING id::text`, rd, cidr)
	f.subnets = append(f.subnets, id)
	return id
}

// approved inserts an approved request directly, as the workflow would leave it.
func (f *fixture) approved(kind, subnet, by string, address *string) string {
	mode := "automatic"
	if address != nil {
		mode = "specific"
	}
	var expiry *string
	if kind == TypeReservation {
		e := "2099-01-01"
		expiry = &e
	}
	var id string
	f.row(&id, `INSERT INTO ip_requests (request_type, subnet_id, selection_mode, requested_address, justification, status,
		requested_by, expiry_date, decided_at) VALUES ($1, $2::uuid, $3, $4::text::inet, 'test', 'approved', $5::uuid, $6::date, now())
		RETURNING id::text`, kind, subnet, mode, address, by, expiry)
	return id
}

func (f *fixture) cleanup() {
	ctx := context.Background()
	err := pgx.BeginFunc(ctx, f.admin, func(tx pgx.Tx) error {
		users, subnets, org := []any{f.users}, []any{f.subnets}, []any{f.org}
		stmts := []struct {
			sql  string
			args []any
		}{
			{`SET LOCAL dfipam.audit_prune = 'on'`, nil},
			{`DELETE FROM audit_logs WHERE actor_id = ANY($1::uuid[])`, users},
			{`DELETE FROM notifications WHERE recipient_id = ANY($1::uuid[])`, users},
			{`DELETE FROM ip_allocations WHERE subnet_id = ANY($1::uuid[])`, subnets},
			{`DELETE FROM ip_reservations WHERE subnet_id = ANY($1::uuid[])`, subnets},
			{`DELETE FROM ip_request_approvals WHERE request_id IN (SELECT id FROM ip_requests WHERE subnet_id = ANY($1::uuid[]))`, subnets},
			{`DELETE FROM ip_requests WHERE subnet_id = ANY($1::uuid[])`, subnets},
			{`DELETE FROM ip_addresses WHERE subnet_id = ANY($1::uuid[])`, subnets},
			{`DELETE FROM subnets WHERE id = ANY($1::uuid[])`, subnets},
			{`DELETE FROM routing_domains WHERE organisation_id = $1::uuid`, org},
			{`DELETE FROM organisations WHERE id = $1::uuid`, org},
			{`DELETE FROM users WHERE id = ANY($1::uuid[])`, users},
		}
		for _, s := range stmts {
			if _, err := tx.Exec(ctx, s.sql, s.args...); err != nil {
				return fmt.Errorf("%s: %w", s.sql, err)
			}
		}
		return nil
	})
	if err != nil {
		f.t.Errorf("cleanup: %v", err)
	}
}

func errorCode(err error) string {
	var e *httpx.Error
	if errors.As(err, &e) {
		return e.Code
	}
	if err != nil {
		return err.Error()
	}
	return ""
}
