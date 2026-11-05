package server

import (
	"testing"

	"dfipam/api/internal/auth"

	"github.com/gin-gonic/gin"
)

// A second run with nothing changed must say so, instead of repeating the same total.
func TestRunReportsOnlyWhatChanged(t *testing.T) {
	p, _ := setupPeople(t)
	c := p.as(p.user("recon-run@example.com", auth.SuperAdminRole))
	org := id(c.must(201, "POST", "/organisations", gin.H{"name": "Run Org", "code": "runorg"}))
	rd := id(c.must(201, "POST", "/routing-domains", gin.H{"organisationId": org, "name": "Run VRF", "kind": "vrf"}))
	c.must(201, "POST", "/subnets", gin.H{"routingDomainId": rd, "cidr": "10.123.1.0/24", "name": "No owner", "environment": "production"})

	first := data(c.must(200, "POST", "/reconciliation/run", nil))
	if first["new"].(float64) < 1 || first["open"].(float64) < 1 {
		t.Fatalf("first run: %v", first)
	}
	second := data(c.must(200, "POST", "/reconciliation/run", nil))
	if second["new"] != float64(0) || second["detected"].(float64) < 1 || second["open"] != first["open"] {
		t.Fatalf("second run: %v", second)
	}
	last := data(c.must(200, "GET", "/reconciliation/last-run", nil))
	if last["ranBy"] != "recon-run@example.com" || last["ranAt"] == nil || last["open"] != first["open"] {
		t.Fatalf("last run: %v", last)
	}
}
