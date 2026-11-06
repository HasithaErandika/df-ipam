package server

import (
	"context"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/allocation"
	"testing"

	"github.com/gin-gonic/gin"
)

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
