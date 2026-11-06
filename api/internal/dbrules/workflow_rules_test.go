package dbrules

import (
	"dfipam/api/internal/platform/db"
	"testing"

	"github.com/jackc/pgx/v5"
)

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

// One address can't be both actively allocated and actively reserved, whichever
// table is written first.
func TestAllocationAndReservationExcludeEachOther(t *testing.T) {
	e := setup(t)
	var user, subnet, ip string
	e.row(&user, `INSERT INTO users (email, auth_provider, auth_subject) VALUES ('x@example.com', 'test', gen_random_uuid()::text) RETURNING id`)
	e.row(&subnet, `INSERT INTO subnets (routing_domain_id, cidr, name, environment) VALUES ($1, '10.4.0.0/24', 's', 'testing') RETURNING id`, e.rd)
	e.row(&ip, `INSERT INTO ip_addresses (routing_domain_id, subnet_id, address) VALUES ($1, $2, '10.4.0.9') RETURNING id`, e.rd, subnet)
	insert := func(table, by string) string {
		return e.try(`INSERT INTO `+table+` (ip_address_id, subnet_id, routing_domain_id, address, `+by+`)
			VALUES ($1, $2, $3, '10.4.0.9', $4)`, ip, subnet, e.rd, user)
	}
	if code := insert("ip_reservations", "reserved_by"); code != "" {
		t.Fatal(code)
	}
	if code := insert("ip_allocations", "allocated_by"); code != db.CodeUnique {
		t.Fatalf("allocation over an active reservation: %q", code)
	}
	if code := e.try(`UPDATE ip_reservations SET status = 'converted' WHERE ip_address_id = $1`, ip); code != "" {
		t.Fatal(code)
	}
	if code := insert("ip_allocations", "allocated_by"); code != "" {
		t.Fatalf("allocation after conversion: %q", code)
	}
	if code := insert("ip_reservations", "reserved_by"); code != db.CodeUnique {
		t.Fatalf("reservation over an active allocation: %q", code)
	}
}

// A person decides at most one step of a request, and a request is fulfilled once.
func TestOneDecisionPerApproverAndOneFulfilment(t *testing.T) {
	e := setup(t)
	var requester, approver, subnet, request string
	e.row(&requester, `INSERT INTO users (email, auth_provider, auth_subject) VALUES ('r@example.com', 'test', gen_random_uuid()::text) RETURNING id`)
	e.row(&approver, `INSERT INTO users (email, auth_provider, auth_subject) VALUES ('a@example.com', 'test', gen_random_uuid()::text) RETURNING id`)
	e.row(&subnet, `INSERT INTO subnets (routing_domain_id, cidr, name, environment) VALUES ($1, '10.3.0.0/24', 's', 'testing') RETURNING id`, e.rd)
	e.row(&request, `INSERT INTO ip_requests (subnet_id, requested_by) VALUES ($1, $2) RETURNING id`, subnet, requester)
	if code := e.try(`INSERT INTO ip_request_approvals (request_id, step_order, approver_id, decision) VALUES ($1, 1, $2, 'approved')`, request, approver); code != "" {
		t.Fatal(code)
	}
	if code := e.try(`INSERT INTO ip_request_approvals (request_id, step_order, approver_id, decision) VALUES ($1, 2, $2, 'approved')`, request, approver); code != db.CodeUnique {
		t.Fatalf("second step by the same person: %q", code)
	}
	var ip string
	e.row(&ip, `INSERT INTO ip_addresses (routing_domain_id, subnet_id, address) VALUES ($1, $2, '10.3.0.5') RETURNING id`, e.rd, subnet)
	alloc := `INSERT INTO ip_allocations (ip_address_id, subnet_id, routing_domain_id, address, request_id, allocated_by, status)
		VALUES ($1, $2, $3, '10.3.0.5', $4, $5, 'released')`
	if code := e.try(alloc, ip, subnet, e.rd, request, approver); code != "" {
		t.Fatal(code)
	}
	if code := e.try(alloc, ip, subnet, e.rd, request, approver); code != db.CodeUnique {
		t.Fatalf("second allocation for one request: %q", code)
	}
	if code := e.try(`INSERT INTO ip_requests (subnet_id, requested_by, request_type) VALUES ($1, $2, 'release')`, subnet, requester); code != db.CodeCheck {
		t.Fatalf("release without a target: %q", code)
	}
}
