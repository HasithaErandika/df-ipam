package allocation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests need committed rows, because concurrency only shows between
// separate connections. Each test builds its own organisation, subnet and users,
// then deletes everything it made, including its audit rows, through the
// privileged retention path. They need TEST_DATABASE_URL (the postgres role);
// the service itself runs as the app role when TEST_APP_DATABASE_URL is set.

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

// Forty automatic allocations, twenty at a time, on one subnet: every one gets
// a different address, with no gaps.
func TestConcurrentAutomaticAllocation(t *testing.T) {
	f := newFixture(t)
	subnet := f.subnet("10.250.0.0/24")
	requester, fulfiller := f.user("req"), f.user("ful")
	var ids []string
	for i := 0; i < 40; i++ {
		ids = append(ids, f.approved(TypeAllocation, subnet, requester.UserID, nil))
	}
	addresses := make([]string, len(ids))
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 20)
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			a, _, err := f.svc.Allocate(context.Background(), fulfiller, id)
			addresses[i], errs[i] = a.Address, err
		}()
	}
	wg.Wait()
	seen := map[string]bool{}
	for i, a := range addresses {
		if errs[i] != nil {
			t.Fatalf("allocation %d: %v", i, errs[i])
		}
		if seen[a] {
			t.Fatalf("address %s given twice", a)
		}
		seen[a] = true
	}
	for i := 1; i <= 40; i++ {
		if !seen[fmt.Sprintf("10.250.0.%d", i)] {
			t.Fatalf("10.250.0.%d was skipped: %v", i, addresses)
		}
	}
}

// More requests than addresses: the subnet fills, and the rest fail cleanly.
func TestConcurrentAllocationExhaustsCleanly(t *testing.T) {
	f := newFixture(t)
	subnet := f.subnet("10.250.1.0/29") // six usable addresses
	requester, fulfiller := f.user("req"), f.user("ful")
	var ids []string
	for i := 0; i < 10; i++ {
		ids = append(ids, f.approved(TypeAllocation, subnet, requester.UserID, nil))
	}
	codes := make([]string, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := f.svc.Allocate(context.Background(), fulfiller, id)
			codes[i] = errorCode(err)
		}()
	}
	wg.Wait()
	ok, exhausted := 0, 0
	for _, c := range codes {
		switch c {
		case "":
			ok++
		case "SUBNET_EXHAUSTED":
			exhausted++
		default:
			t.Fatalf("unexpected result %q", c)
		}
	}
	if ok != 6 || exhausted != 4 {
		t.Fatalf("got %d allocated, %d exhausted", ok, exhausted)
	}
}

// Allocations and reservations racing for one chosen address: exactly one wins,
// and the address is never both allocated and reserved.
func TestConcurrentAllocationAndReservationOfOneAddress(t *testing.T) {
	f := newFixture(t)
	subnet := f.subnet("10.250.2.0/24")
	requester, fulfiller := f.user("req"), f.user("ful")
	addr := "10.250.2.77"
	type job struct{ kind, id string }
	var jobs []job
	for i := 0; i < 10; i++ {
		jobs = append(jobs, job{TypeAllocation, f.approved(TypeAllocation, subnet, requester.UserID, &addr)})
		jobs = append(jobs, job{TypeReservation, f.approved(TypeReservation, subnet, requester.UserID, &addr)})
	}
	codes := make([]string, len(jobs))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			var err error
			if j.kind == TypeAllocation {
				_, _, err = f.svc.Allocate(context.Background(), fulfiller, j.id)
			} else {
				_, _, err = f.svc.Reserve(context.Background(), fulfiller, j.id)
			}
			codes[i] = errorCode(err)
		}()
	}
	close(start)
	wg.Wait()
	wins := 0
	for _, c := range codes {
		switch c {
		case "":
			wins++
		case "ADDRESS_UNAVAILABLE":
		default:
			t.Fatalf("unexpected result %q", c)
		}
	}
	if wins != 1 {
		t.Fatalf("%d claims won, want exactly 1: %v", wins, codes)
	}
	var allocations, reservations int
	f.row(&allocations, `SELECT count(*) FROM ip_allocations WHERE subnet_id = $1::uuid AND status = 'active'`, subnet)
	f.row(&reservations, `SELECT count(*) FROM ip_reservations WHERE subnet_id = $1::uuid AND status = 'active'`, subnet)
	if allocations+reservations != 1 {
		t.Fatalf("%d active allocations and %d active reservations", allocations, reservations)
	}
}

// Two approvers deciding the same step at once: one decision counts.
func TestConcurrentApprovalOfOneStep(t *testing.T) {
	f := newFixture(t)
	subnet := f.subnet("10.250.3.0/24")
	requester, a1, a2 := f.user("req"), f.user("a1"), f.user("a2")
	var request, step string
	f.row(&request, `INSERT INTO ip_requests (request_type, subnet_id, justification, status, requested_by, submitted_at)
		VALUES ('allocation', $1::uuid, 'test', 'pending_approval', $2::uuid, now()) RETURNING id::text`, subnet, requester.UserID)
	f.row(&step, `INSERT INTO ip_request_approvals (request_id, step_order) VALUES ($1::uuid, 1) RETURNING id::text`, request)
	codes := make([]string, 2)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i, who := range []*auth.Principal{a1, a2} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := f.svc.Decide(context.Background(), who, step, i == 0, "racing")
			codes[i] = errorCode(err)
		}()
	}
	close(start)
	wg.Wait()
	if (codes[0] == "") == (codes[1] == "") || (codes[0] != "" && codes[0] != "APPROVAL_NOT_CURRENT") || (codes[1] != "" && codes[1] != "APPROVAL_NOT_CURRENT") {
		t.Fatalf("want one decision and one APPROVAL_NOT_CURRENT, got %q", codes)
	}
	var decided int
	f.row(&decided, `SELECT count(*) FROM ip_request_approvals WHERE request_id = $1::uuid AND decision <> 'pending'`, request)
	if decided != 1 {
		t.Fatalf("%d decisions recorded", decided)
	}
}

// Repeating a fulfilment at the same moment, as a retried HTTP call would,
// produces one allocation.
func TestConcurrentRepeatedFulfilment(t *testing.T) {
	f := newFixture(t)
	subnet := f.subnet("10.250.4.0/24")
	requester, fulfiller := f.user("req"), f.user("ful")
	id := f.approved(TypeAllocation, subnet, requester.UserID, nil)
	results := make([]string, 8)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, _, err := f.svc.Allocate(context.Background(), fulfiller, id)
			if err != nil {
				results[i] = "error: " + err.Error()
				return
			}
			results[i] = a.ID
		}()
	}
	wg.Wait()
	for _, r := range results[1:] {
		if r != results[0] {
			t.Fatalf("repeats disagree: %v", results)
		}
	}
	var n int
	f.row(&n, `SELECT count(*) FROM ip_allocations WHERE request_id = $1::uuid`, id)
	if n != 1 {
		t.Fatalf("%d allocations for one request", n)
	}
}
