package server

import (
	"testing"

	"dfipam/api/internal/auth"

	"github.com/gin-gonic/gin"
)

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
