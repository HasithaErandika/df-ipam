package server

import (
	"context"
	"testing"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/reconciliation"

	"github.com/gin-gonic/gin"
)

func TestChangeRequestsNeedASecondPerson(t *testing.T) {
	p, tx := setupPeople(t)
	ctx := context.Background()
	proposer := p.user("p5a@example.com", "IPAM Administrator")
	approver := p.user("p5b@example.com", "IPAM Administrator")
	c := p.as(proposer)

	org := id(c.must(201, "POST", "/organisations", gin.H{"name": "CR Org", "code": "cr"}))
	rd := id(c.must(201, "POST", "/routing-domains", gin.H{"organisationId": org, "name": "CR VRF", "kind": "vrf"}))
	subnet := id(c.must(201, "POST", "/subnets", gin.H{"routingDomainId": rd, "cidr": "10.56.0.0/24", "name": "CR", "environment": "production"}))
	if _, err := reconciliation.Run(ctx, tx, proposer.UserID); err != nil {
		t.Fatal(err)
	}
	finding := text(t, tx, `SELECT id::text FROM reconciliation_results WHERE classification = 'ownership_missing' AND object_id = $1::uuid`, subnet)
	path := "/reconciliation/results/" + finding + "/change-requests"

	c.must(422, "POST", path, gin.H{"changes": gin.H{"technicalOwnerId": approver.UserID}})
	// Allocation state can never travel through a change request.
	c.must(422, "POST", path, gin.H{"changes": gin.H{"allocationState": "allocated"}, "reason": "x"})
	c.must(422, "POST", path, gin.H{"changes": gin.H{"cidr": "10.0.0.0/8"}, "reason": "x"})
	change := id(c.must(201, "POST", path, gin.H{"changes": gin.H{"technicalOwnerId": approver.UserID}, "reason": "Network team owns this"}))
	if out := c.must(409, "POST", path, gin.H{"changes": gin.H{"technicalOwnerId": proposer.UserID}, "reason": "again"}); errCode(out) != "CHANGE_ALREADY_OPEN" {
		t.Fatalf("second: %v", out)
	}

	// Proposing changes nothing yet, and the proposer can't approve it.
	if n := count(t, tx, `SELECT count(*) FROM subnets WHERE id = $1::uuid AND technical_owner_id IS NOT NULL`, subnet); n != 0 {
		t.Fatal("a proposal changed inventory")
	}
	if out := c.must(403, "POST", "/change-requests/"+change+"/approve", gin.H{}); errCode(out) != "SELF_APPROVAL_DENIED" {
		t.Fatalf("self approval: %v", out)
	}

	a := p.as(approver)
	a.must(422, "POST", "/change-requests/"+change+"/reject", gin.H{})
	out := data(a.must(200, "POST", "/change-requests/"+change+"/approve", gin.H{"note": "Agreed"}))
	if out["status"] != "applied" {
		t.Fatalf("approve: %v", out)
	}
	if got := text(t, tx, `SELECT technical_owner_id::text FROM subnets WHERE id = $1::uuid`, subnet); got != approver.UserID {
		t.Fatalf("owner: %s", got)
	}
	if got := text(t, tx, `SELECT review_status FROM reconciliation_results WHERE id = $1::uuid`, finding); got != "accepted" {
		t.Fatalf("finding: %s", got)
	}
	if n := count(t, tx, `SELECT count(*) FROM audit_logs WHERE action = 'subnet.change_applied' AND object_id = $1 AND approval_ref = $2 AND actor_id = $3::uuid`, subnet, change, approver.UserID); n != 1 {
		t.Fatal("no audit row ties the change to its approval")
	}
	if out := a.must(409, "POST", "/change-requests/"+change+"/approve", gin.H{}); errCode(out) != "CHANGE_ALREADY_DECIDED" {
		t.Fatalf("repeat: %v", out)
	}
	// The database stops self-decision even if the service had a bug.
	_, err := tx.Exec(ctx, `SAVEPOINT s`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE change_requests SET decided_by = requested_by WHERE id = $1::uuid`, change); err == nil {
		t.Fatal("the constraint let someone decide their own change")
	}
	if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT s`); err != nil {
		t.Fatal(err)
	}
}

func TestRuleSettings(t *testing.T) {
	p, tx := setupPeople(t)
	ctx := context.Background()
	admin := p.user("rules@example.com", auth.SuperAdminRole)
	c := p.as(admin)
	org := id(c.must(201, "POST", "/organisations", gin.H{"name": "Rule Org", "code": "ru"}))
	rd := id(c.must(201, "POST", "/routing-domains", gin.H{"organisationId": org, "name": "Rule VRF", "kind": "vrf"}))
	subnet := id(c.must(201, "POST", "/subnets", gin.H{"routingDomainId": rd, "cidr": "10.57.0.0/24", "name": "Rule", "environment": "testing"}))
	if _, err := reconciliation.Run(ctx, tx, admin.UserID); err != nil {
		t.Fatal(err)
	}
	c.must(422, "PUT", "/reconciliation/rules/unknown_ip", gin.H{"isEnabled": true, "weights": gin.H{"reliability": 0.9, "freshness": 0.9, "agreement": 0, "cap": 50, "freshnessDays": 7}})
	c.must(422, "PUT", "/reconciliation/rules/unknown_ip", gin.H{"isEnabled": true, "sourcePriority": []string{"Nonsense"}})
	c.must(404, "PUT", "/reconciliation/rules/nonsense", gin.H{"isEnabled": true})
	// Switching a finding off stops detection, and doesn't pretend the problem is fixed.
	c.must(200, "PUT", "/reconciliation/rules/ownership_missing", gin.H{"isEnabled": false})
	if _, err := tx.Exec(ctx, `UPDATE reconciliation_results SET last_detected_at = now() - interval '1 day' WHERE object_id = $1::uuid`, subnet); err != nil {
		t.Fatal(err)
	}
	if _, err := reconciliation.Run(ctx, tx, admin.UserID); err != nil {
		t.Fatal(err)
	}
	if got := text(t, tx, `SELECT review_status FROM reconciliation_results WHERE object_id = $1::uuid AND classification = 'ownership_missing'`, subnet); got != "open" {
		t.Fatalf("status: %s", got)
	}
}
