package server

import (
	"fmt"
	"testing"

	"dfipam/api/internal/auth"

	"github.com/gin-gonic/gin"
)

func etag(v float64) string { return fmt.Sprintf(`"%d"`, int(v)) }

type scopeWorld struct {
	domain, lending, cards, lendingNet, cardsNet, lendingIP, cardsIP string
}

func scopedNetwork(t *testing.T, c client) scopeWorld {
	t.Helper()
	org := id(c.must(201, "POST", "/organisations", gin.H{"name": "Scope Org", "code": "SCP"}))
	w := scopeWorld{domain: id(c.must(201, "POST", "/routing-domains", gin.H{"organisationId": org, "name": "Scope VRF", "kind": "vrf"}))}
	w.lending = id(c.must(201, "POST", "/business-services", gin.H{"name": "Scope Lending"}))
	w.cards = id(c.must(201, "POST", "/business-services", gin.H{"name": "Scope Cards"}))
	add := func(cidr, service string) string {
		return id(c.must(201, "POST", "/subnets", gin.H{"routingDomainId": w.domain, "cidr": cidr, "name": "Net " + cidr, "environment": "production",
			"gateway": "10.93." + cidr[6:7] + ".1", "businessServiceId": service}))
	}
	w.lendingNet, w.cardsNet = add("10.93.1.0/24", w.lending), add("10.93.2.0/24", w.cards)
	w.lendingIP = id(c.must(201, "POST", "/ip-addresses", gin.H{"routingDomainId": w.domain, "address": "10.93.1.5", "mac": "00:11:22:33:44:55"}))
	w.cardsIP = id(c.must(201, "POST", "/ip-addresses", gin.H{"routingDomainId": w.domain, "address": "10.93.2.5"}))
	return w
}

func TestBusinessViewerSeesOnlyTheirServices(t *testing.T) {
	p, _ := setupPeople(t)
	root := p.user("scope-root@example.com", auth.SuperAdminRole)
	w := scopedNetwork(t, p.as(root))
	person := p.user("scope-viewer@example.com", viewer)

	// With no business services, a scoped view is empty, never unfiltered.
	c := p.as(person)
	if n := total(t, c, "/subnets"); n != 0 {
		t.Fatalf("no scope should see nothing: %v", n)
	}
	p.as(root).must(204, "PUT", "/users/"+person.UserID+"/scope", gin.H{"businessServiceIds": []string{w.lending}})

	c = p.as(person)
	if n := total(t, c, "/subnets"); n != 1 {
		t.Fatalf("subnets in scope: %v", n)
	}
	if n := total(t, c, "/ip-addresses"); n != 1 {
		t.Fatalf("addresses in scope: %v", n)
	}
	subnet := data(c.must(200, "GET", "/subnets/"+w.lendingNet, nil))
	if subnet["gateway"] != nil || subnet["vlan"] != nil {
		t.Fatalf("infrastructure leaked: %v", subnet)
	}
	if a := data(c.must(200, "GET", "/ip-addresses/"+w.lendingIP, nil)); a["mac"] != nil || a["deviceId"] != nil {
		t.Fatalf("infrastructure leaked: %v", a)
	}
	// A hidden field can't be probed through search.
	if n := total(t, c, "/ip-addresses?q=00:11:22"); n != 0 {
		t.Fatalf("MAC search answered a scoped reader: %v", n)
	}
	// Out of scope reads as missing, and the rest of the inventory stays closed.
	c.must(404, "GET", "/subnets/"+w.cardsNet, nil)
	c.must(404, "GET", "/ip-addresses/"+w.cardsIP, nil)
	c.must(403, "GET", "/subnets/"+w.lendingNet+"/addresses", nil)
	c.must(403, "GET", "/ip-addresses/"+w.lendingIP+"/evidence", nil)
	c.must(403, "GET", "/devices", nil)
	c.must(403, "GET", "/search?q=10.93", nil)

	// Keeping the role keeps its scope, and taking it away removes it.
	var role, other string
	tx := p.tx
	if err := tx.QueryRow(t.Context(), `SELECT id::text FROM roles WHERE name = $1`, viewer).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(t.Context(), `SELECT id::text FROM roles WHERE name = $1`, auditor).Scan(&other); err != nil {
		t.Fatal(err)
	}
	p.as(root).must(204, "PUT", "/users/"+person.UserID+"/roles", gin.H{"roleIds": []string{role}})
	if n := total(t, p.as(person), "/subnets"); n != 1 {
		t.Fatalf("scope lost when the role was kept: %v", n)
	}
	p.as(root).must(204, "PUT", "/users/"+person.UserID+"/roles", gin.H{"roleIds": []string{other}})
	if n := count(t, tx, `SELECT count(*) FROM user_roles WHERE user_id = $1::uuid AND business_service_id IS NOT NULL`, person.UserID); n != 0 {
		t.Fatalf("scope stayed after the role went: %d", n)
	}
	p.as(root).must(422, "PUT", "/users/"+person.UserID+"/scope", gin.H{"businessServiceIds": []string{w.lending}})
	p.as(root).must(403, "PUT", "/users/"+root.UserID+"/scope", gin.H{"businessServiceIds": []string{}})
}

func TestOwnersEditOnlyWhatIsAssigned(t *testing.T) {
	p, _ := setupPeople(t)
	root := p.user("own-root@example.com", auth.SuperAdminRole)
	engineer := p.user("own-eng@example.com", infra)
	r := p.as(root)
	w := scopedNetwork(t, r)
	mine := w.lendingIP
	r.must(200, "PATCH", "/ip-addresses/"+mine, gin.H{"ownerId": engineer.UserID})
	version := data(r.must(200, "GET", "/subnets/"+w.lendingNet, nil))["version"].(float64)
	owned := data(r.must(200, "PATCH", "/subnets/"+w.lendingNet, gin.H{"technicalOwnerId": engineer.UserID}, "If-Match", etag(version)))
	device := id(r.must(201, "POST", "/devices", gin.H{"name": "own-fw", "ownerId": engineer.UserID}))
	others := id(r.must(201, "POST", "/devices", gin.H{"name": "other-fw"}))

	c := p.as(engineer)
	if a := data(c.must(200, "PATCH", "/ip-addresses/"+mine, gin.H{"hostname": "mine-01"})); a["hostname"] != "mine-01" {
		t.Fatalf("owner edit: %v", a)
	}
	c.must(403, "PATCH", "/ip-addresses/"+w.cardsIP, gin.H{"hostname": "nope"})
	c.must(403, "PATCH", "/ip-addresses/"+mine, gin.H{"ownerId": root.UserID})
	c.must(403, "PATCH", "/ip-addresses/"+mine, gin.H{"allocationState": "allocated"})

	tag := etag(owned["version"].(float64))
	c.must(403, "PATCH", "/subnets/"+w.lendingNet, gin.H{"discoveryEnabled": true}, "If-Match", tag)
	c.must(403, "PATCH", "/subnets/"+w.lendingNet, gin.H{"technicalOwnerId": root.UserID}, "If-Match", tag)
	c.must(200, "PATCH", "/subnets/"+w.lendingNet, gin.H{"description": "Looked after by its owner"}, "If-Match", tag)
	cards := data(r.must(200, "GET", "/subnets/"+w.cardsNet, nil))["version"].(float64)
	c.must(403, "PATCH", "/subnets/"+w.cardsNet, gin.H{"description": "nope"}, "If-Match", etag(cards))

	c.must(200, "PATCH", "/devices/"+device, gin.H{"notes": "Mine"})
	c.must(403, "PATCH", "/devices/"+device, gin.H{"ownerId": root.UserID})
	c.must(403, "PATCH", "/devices/"+others, gin.H{"notes": "nope"})
	if n := count(t, p.tx, `SELECT count(*) FROM audit_logs WHERE actor_id = $1::uuid AND action IN ('ip_address.update','subnet.update','device.update')`, engineer.UserID); n != 3 {
		t.Fatalf("owner edits should be audited: %d", n)
	}
}

func TestDashboardSaysNoMoreThanThePages(t *testing.T) {
	p, tx := setupPeople(t)
	if _, err := tx.Exec(t.Context(), `INSERT INTO discovery_agents (name, status, last_heartbeat_at) VALUES ('dash-agent', 'active', now())`); err != nil {
		t.Fatal(err)
	}
	agents := func(role string) any {
		return data(p.as(p.user(role+"@dash.example.com", role)).must(200, "GET", "/dashboard/summary", nil))["agentsOnline"]
	}
	if agents(auditor) == nil {
		t.Fatal("an auditor reads discovery")
	}
	if got := agents(infra); got != nil {
		t.Fatalf("agents shown to a role that can't open discovery: %v", got)
	}
}
