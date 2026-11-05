package server

import (
	"context"
	"os"
	"testing"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/allocation"
	"dfipam/api/internal/seed"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// actingResolver lets one test act as several people.
type actingResolver struct{ current **auth.Principal }

func (a actingResolver) Resolve(context.Context, auth.Identity) (*auth.Principal, error) {
	return *a.current, nil
}

type people struct {
	t       *testing.T
	tx      pgx.Tx
	current *auth.Principal
	client  client
}

func (p *people) user(email string, roles ...string) *auth.Principal {
	p.t.Helper()
	ctx := context.Background()
	var id string
	if err := p.tx.QueryRow(ctx, `INSERT INTO users (email, auth_provider, auth_subject) VALUES ($1, 'test', gen_random_uuid()::text) RETURNING id::text`, email).Scan(&id); err != nil {
		p.t.Fatal(err)
	}
	perms := map[string]bool{}
	for _, role := range roles {
		if _, err := p.tx.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) SELECT $1::uuid, id FROM roles WHERE name = $2`, id, role); err != nil {
			p.t.Fatal(err)
		}
		for _, r := range seed.Roles {
			if r.Name == role {
				for _, perm := range r.Permissions {
					perms[perm] = true
				}
			}
		}
	}
	return &auth.Principal{UserID: id, Email: email, Roles: roles, Permissions: perms}
}

func (p *people) as(u *auth.Principal) client { p.current = u; return p.client }

func setupPeople(t *testing.T) (*people, pgx.Tx) {
	t.Helper()
	url := os.Getenv("TEST_APP_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_APP_DATABASE_URL is not set")
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
	p := &people{t: t, tx: tx}
	gin.SetMode(gin.TestMode)
	engine, _ := New(Deps{Conn: tx, Verifier: stubVerifier{}, Resolver: actingResolver{&p.current}, Keys: testKeys(t), Outbound: testPolicy})
	p.client = client{t, engine}
	return p, tx
}

func count(t *testing.T, tx pgx.Tx, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := tx.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func errCode(m map[string]any) string {
	e, _ := m["error"].(map[string]any)
	s, _ := e["code"].(string)
	return s
}

func TestAllocationWorkflow(t *testing.T) {
	p, tx := setupPeople(t)
	admin := p.user("admin@example.com", auth.SuperAdminRole)
	requester := p.user("requester@example.com", "Infrastructure Engineer")
	approver := p.user("approver@example.com", "IPAM Administrator")
	second := p.user("second@example.com", "IPAM Administrator")
	engineer := p.user("engineer@example.com", "Network Engineer")

	c := p.as(admin)
	org := id(c.must(201, "POST", "/organisations", gin.H{"name": "WF Org", "code": "wf"}))
	rd := id(c.must(201, "POST", "/routing-domains", gin.H{"organisationId": org, "name": "WF", "kind": "vrf"}))
	subnet := id(c.must(201, "POST", "/subnets", gin.H{"routingDomainId": rd, "cidr": "10.70.0.0/29", "name": "WF net",
		"environment": "testing", "gateway": "10.70.0.1"}))

	// The requester drafts, edits and submits an automatic allocation.
	c = p.as(requester)
	draft := data(c.must(201, "POST", "/requests", gin.H{"requestType": "allocation", "subnetId": subnet, "justification": "App server"}))
	rid := draft["id"].(string)
	if draft["status"] != "draft" {
		t.Fatalf("status %v", draft["status"])
	}
	c.must(412, "PATCH", "/requests/"+rid, gin.H{"requestType": "allocation", "subnetId": subnet, "justification": "x"}, "If-Match", "99")
	c.must(200, "PATCH", "/requests/"+rid, gin.H{"requestType": "allocation", "subnetId": subnet, "justification": "App server 2"}, "If-Match", "1")
	submitted := data(c.must(200, "POST", "/requests/"+rid+"/submit", nil))
	if submitted["status"] != "pending_approval" || len(submitted["approvals"].([]any)) != 1 {
		t.Fatalf("submitted: %v", submitted)
	}
	c.must(409, "PATCH", "/requests/"+rid, gin.H{"requestType": "allocation", "subnetId": subnet, "justification": "late"}, "If-Match", "3")
	// An infrastructure engineer can't approve, allocate or see other people's requests.
	c.must(403, "GET", "/approvals", nil)
	c.must(403, "POST", "/allocations", gin.H{"requestId": rid})

	// Approvers were told.
	if n := count(t, tx, `SELECT count(*) FROM notifications WHERE recipient_id = $1::uuid AND event_type = 'approval_requested'`, approver.UserID); n != 2 {
		t.Fatalf("approver notifications (in-app and email): %d", n)
	}
	if n := count(t, tx, `SELECT count(*) FROM notifications WHERE recipient_id = $1::uuid`, requester.UserID); n != 0 {
		t.Fatalf("requester notified about their own submission: %d", n)
	}

	// Nobody approves their own request, even holding the permission.
	selfReq := data(p.as(approver).must(201, "POST", "/requests", gin.H{"requestType": "allocation", "subnetId": subnet, "justification": "mine", "submit": true}))
	selfStep := selfReq["approvals"].([]any)[0].(map[string]any)["id"].(string)
	if code, out := p.as(approver).call("POST", "/approvals/"+selfStep+"/approve", nil); code != 403 || errCode(out) != "SELF_APPROVAL_DENIED" {
		t.Fatalf("self-approval: %d %v", code, out)
	}
	pending := p.as(approver).must(200, "GET", "/approvals", nil)["data"].([]any)
	if len(pending) != 1 || pending[0].(map[string]any)["requestId"] != rid {
		t.Fatalf("approver's queue should hold only the other request: %v", pending)
	}

	step := submitted["approvals"].([]any)[0].(map[string]any)["id"].(string)
	p.as(approver).must(422, "POST", "/approvals/"+step+"/reject", gin.H{"comment": " "})
	approved := data(p.as(approver).must(200, "POST", "/approvals/"+step+"/approve", gin.H{"comment": "ok"}))
	if approved["status"] != "approved" {
		t.Fatalf("approved: %v", approved)
	}
	if code, out := p.as(second).call("POST", "/approvals/"+step+"/approve", nil); code != 409 || errCode(out) != "APPROVAL_NOT_CURRENT" {
		t.Fatalf("decided twice: %d %v", code, out)
	}

	// Fulfilment picks the first candidate past the gateway, and repeats are safe.
	c = p.as(engineer)
	alloc := data(c.must(201, "POST", "/allocations", gin.H{"requestId": rid}))
	if alloc["address"] != "10.70.0.2" || alloc["status"] != "active" {
		t.Fatalf("allocation: %v", alloc)
	}
	again := data(c.must(200, "POST", "/allocations", gin.H{"requestId": rid}))
	if again["id"] != alloc["id"] {
		t.Fatal("repeat created a second allocation")
	}
	if n := count(t, tx, `SELECT count(*) FROM ip_addresses WHERE subnet_id = $1::uuid AND allocation_state = 'allocated'`, subnet); n != 1 {
		t.Fatalf("allocated records: %d", n)
	}
	if n := count(t, tx, `SELECT count(*) FROM audit_logs WHERE action = 'ip_allocation.create' AND approval_ref = $1`, rid); n != 1 {
		t.Fatalf("audit rows: %d", n)
	}
	fulfilled := data(p.as(requester).must(200, "GET", "/requests/"+rid, nil))
	if fulfilled["status"] != "allocated" || fulfilled["result"].(map[string]any)["address"] != "10.70.0.2" {
		t.Fatalf("request after fulfilment: %v", fulfilled)
	}
	p.as(requester).must(404, "GET", "/requests/"+selfReq["id"].(string), nil)

	// A specific request for the allocated address is refused up front.
	if code, out := p.as(requester).call("POST", "/requests", gin.H{"requestType": "allocation", "subnetId": subnet,
		"selectionMode": "specific", "requestedAddress": "10.70.0.2", "justification": "dup"}); code != 409 || errCode(out) != "ADDRESS_UNAVAILABLE" {
		t.Fatalf("duplicate specific: %d %v", code, out)
	}

	_ = second
}

func TestReservationLifecycle(t *testing.T) {
	p, tx := setupPeople(t)
	admin := p.user("admin2@example.com", auth.SuperAdminRole)
	requester := p.user("req2@example.com", "Infrastructure Engineer")
	a1 := p.user("a1@example.com", "IPAM Administrator")
	a2 := p.user("a2@example.com", "IPAM Administrator")

	c := p.as(admin)
	org := id(c.must(201, "POST", "/organisations", gin.H{"name": "RS Org", "code": "rs"}))
	rd := id(c.must(201, "POST", "/routing-domains", gin.H{"organisationId": org, "name": "RS", "kind": "vrf"}))
	subnet := id(c.must(201, "POST", "/subnets", gin.H{"routingDomainId": rd, "cidr": "10.71.0.0/24", "name": "RS net", "environment": "uat"}))
	var ipamRole string
	tx.QueryRow(context.Background(), `SELECT id::text FROM roles WHERE name = 'IPAM Administrator'`).Scan(&ipamRole)
	c.must(201, "POST", "/approval-workflows", gin.H{"name": "Two-step reservations", "matchRules": gin.H{"requestTypes": []string{"reservation"}},
		"steps": []gin.H{{"approverRoleId": ipamRole}, {"approverRoleId": ipamRole}}})

	expiry := "2027-06-30"
	res := data(p.as(requester).must(201, "POST", "/requests", gin.H{"requestType": "reservation", "subnetId": subnet, "selectionMode": "specific",
		"requestedAddress": "10.71.0.50", "expiryDate": expiry, "justification": "migration", "submit": true}))
	rid := res["id"].(string)
	steps := res["approvals"].([]any)
	if res["workflowName"] != "Two-step reservations" || len(steps) != 2 {
		t.Fatalf("workflow not applied: %v", res)
	}
	first, second := steps[0].(map[string]any)["id"].(string), steps[1].(map[string]any)["id"].(string)
	if code, out := p.as(a1).call("POST", "/approvals/"+second+"/approve", nil); code != 409 || errCode(out) != "APPROVAL_NOT_CURRENT" {
		t.Fatalf("out of order: %d %v", code, out)
	}
	p.as(a1).must(200, "POST", "/approvals/"+first+"/approve", nil)
	if code, out := p.as(a1).call("POST", "/approvals/"+second+"/approve", nil); code != 403 || errCode(out) != "SEPARATION_OF_DUTIES" {
		t.Fatalf("two steps by one person: %d %v", code, out)
	}
	p.as(a1).must(409, "POST", "/reservations", gin.H{"requestId": rid}) // not fully approved yet
	p.as(a2).must(200, "POST", "/approvals/"+second+"/approve", nil)

	rv := data(p.as(a1).must(201, "POST", "/reservations", gin.H{"requestId": rid}))
	rvID := rv["id"].(string)
	if rv["address"] != "10.71.0.50" || rv["status"] != "active" {
		t.Fatalf("reservation: %v", rv)
	}
	// Extending needs a later date and a reason.
	p.as(a1).must(422, "POST", "/reservations/"+rvID+"/extend", gin.H{"expiryDate": "2027-07-30"})
	p.as(a1).must(422, "POST", "/reservations/"+rvID+"/extend", gin.H{"expiryDate": "2027-01-01", "reason": "earlier"})

	// Converting the reservation into an allocation keeps the address.
	conv := data(p.as(requester).must(201, "POST", "/requests", gin.H{"requestType": "allocation", "reservationId": rvID,
		"justification": "go live", "submit": true}))
	step := conv["approvals"].([]any)[0].(map[string]any)["id"].(string)
	p.as(a2).must(200, "POST", "/approvals/"+step+"/approve", nil)
	alloc := data(p.as(a1).must(201, "POST", "/allocations", gin.H{"requestId": conv["id"]}))
	if alloc["address"] != "10.71.0.50" {
		t.Fatalf("converted allocation: %v", alloc)
	}
	if got := data(p.as(a1).must(200, "GET", "/reservations/"+rvID, nil))["status"]; got != "converted" {
		t.Fatalf("reservation after conversion: %v", got)
	}

	// Releasing goes through approval too, and frees the address.
	rel := data(p.as(requester).must(201, "POST", "/requests", gin.H{"requestType": "release", "allocationId": alloc["id"],
		"justification": "decommissioned", "submit": true}))
	if code, out := p.as(requester).call("POST", "/requests", gin.H{"requestType": "release", "allocationId": alloc["id"], "justification": "again"}); code != 409 || errCode(out) != "RELEASE_ALREADY_REQUESTED" {
		t.Fatalf("second release request: %d %v", code, out)
	}
	p.as(a1).must(409, "POST", "/allocations/"+alloc["id"].(string)+"/release", gin.H{"requestId": rel["id"]})
	relStep := rel["approvals"].([]any)[0].(map[string]any)["id"].(string)
	p.as(a2).must(200, "POST", "/approvals/"+relStep+"/approve", nil)
	released := data(p.as(a1).must(200, "POST", "/allocations/"+alloc["id"].(string)+"/release", gin.H{"requestId": rel["id"]}))
	if released["status"] != "released" {
		t.Fatalf("release: %v", released)
	}
	if n := count(t, tx, `SELECT count(*) FROM ip_addresses WHERE subnet_id = $1::uuid AND address = '10.71.0.50' AND allocation_state = 'available_candidate' AND owner_id IS NULL`, subnet); n != 1 {
		t.Fatal("address not returned to the candidate pool")
	}

	// A second reservation, expired by the worker job.
	res2 := data(p.as(requester).must(201, "POST", "/requests", gin.H{"requestType": "reservation", "subnetId": subnet,
		"expiryDate": "2027-01-31", "justification": "short", "submit": true}))
	for _, s := range res2["approvals"].([]any) {
		approver := a1
		if s.(map[string]any)["stepOrder"].(float64) == 2 {
			approver = a2
		}
		p.as(approver).must(200, "POST", "/approvals/"+s.(map[string]any)["id"].(string)+"/approve", nil)
	}
	rv2 := data(p.as(a1).must(201, "POST", "/reservations", gin.H{"requestId": res2["id"]}))
	if rv2["address"] != "10.71.0.1" {
		t.Fatalf("automatic reservation: %v", rv2)
	}
	ctx := context.Background()
	if _, err := tx.Exec(ctx, `UPDATE ip_reservations SET expires_at = now() + interval '2 days' WHERE id = $1::uuid`, rv2["id"]); err != nil {
		t.Fatal(err)
	}
	svc := allocation.NewService(tx)
	out, err := svc.RunJobs(ctx)
	if err != nil || out.RemindersSent != 1 || out.ReservationsExpired != 0 {
		t.Fatalf("reminder run: %+v %v", out, err)
	}
	if out, _ := svc.RunJobs(ctx); out.RemindersSent != 0 {
		t.Fatal("reminder sent twice")
	}
	if _, err := tx.Exec(ctx, `UPDATE ip_reservations SET expires_at = now() - interval '1 minute' WHERE id = $1::uuid`, rv2["id"]); err != nil {
		t.Fatal(err)
	}
	if out, err := svc.RunJobs(ctx); err != nil || out.ReservationsExpired != 1 {
		t.Fatalf("expiry run: %+v %v", out, err)
	}
	if n := count(t, tx, `SELECT count(*) FROM ip_addresses WHERE address = '10.71.0.1' AND subnet_id = $1::uuid AND allocation_state = 'available_candidate'`, subnet); n != 1 {
		t.Fatal("expired reservation still holds its address")
	}
	if n := count(t, tx, `SELECT count(*) FROM audit_logs WHERE action = 'ip_reservation.expire' AND actor_type = 'system' AND object_id = $1`, rv2["id"]); n != 1 {
		t.Fatal("expiry not audited as the system")
	}

	// The requester sees their notifications and can mark them read.
	rc := p.as(requester)
	list := rc.must(200, "GET", "/notifications?unread=true", nil)
	if list["meta"].(map[string]any)["total"].(float64) < 4 {
		t.Fatalf("requester notifications: %v", list["meta"])
	}
	nid := list["data"].([]any)[0].(map[string]any)["id"].(string)
	p.as(a1).must(404, "POST", "/notifications/"+nid+"/read", nil)
	rc = p.as(requester)
	rc.must(204, "POST", "/notifications/"+nid+"/read", nil)
	rc.must(204, "POST", "/notifications/read-all", nil)
	if got := data(rc.must(200, "GET", "/notifications/unread-count", nil))["unread"]; got != float64(0) {
		t.Fatalf("unread after read-all: %v", got)
	}
	rc.must(200, "PUT", "/notifications/preferences", []gin.H{{"eventType": "request_fulfilled", "channel": "email", "isEnabled": false}})
	rc.must(422, "PUT", "/notifications/preferences", []gin.H{{"eventType": "nope", "channel": "email", "isEnabled": false}})
	rc.must(403, "GET", "/notifications/rules", nil)
	if n := count(t, tx, `SELECT count(*) FROM notification_preferences WHERE user_id = $1::uuid AND NOT is_enabled`, requester.UserID); n != 1 {
		t.Fatalf("preference not stored: %d", n)
	}
	p.as(admin).must(200, "PUT", "/notifications/rules/request_fulfilled", gin.H{"isEnabled": false})
	p.as(admin).must(404, "PUT", "/notifications/rules/nope", gin.H{"isEnabled": false})
}
