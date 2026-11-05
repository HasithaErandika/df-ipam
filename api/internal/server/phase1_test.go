package server

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func rows(m map[string]any) []any       { return m["data"].([]any) }
func total(m map[string]any) float64    { return m["meta"].(map[string]any)["total"].(float64) }
func errorCode(m map[string]any) string { return m["error"].(map[string]any)["code"].(string) }

func TestCoreIPAMFlow(t *testing.T) {
	c := setup(t)

	// Organisations and routing domains can be edited, and only deleted when unused.
	org := id(c.must(201, "POST", "/organisations", gin.H{"name": "Core Org", "code": "core"}))
	if got := data(c.must(200, "PATCH", "/organisations/"+org, gin.H{"name": "Core Organisation", "code": "cor"})); got["code"] != "COR" {
		t.Fatalf("org: %v", got)
	}
	empty := id(c.must(201, "POST", "/organisations", gin.H{"name": "Empty Org", "code": "empty"}))
	c.must(204, "DELETE", "/organisations/"+empty, nil)
	c.must(404, "GET", "/organisations/"+empty, nil)

	vpc := id(c.must(201, "POST", "/routing-domains", gin.H{"organisationId": org, "name": "Prod VPC", "kind": "vpc"}))
	rd := id(c.must(201, "POST", "/routing-domains", gin.H{"organisationId": org, "name": "Core VRF", "kind": "vrf"}))
	if got := data(c.must(200, "PATCH", "/routing-domains/"+rd, gin.H{"routeDistinguisher": "65000:10", "isIsolated": true})); got["routeDistinguisher"] != "65000:10" || got["isIsolated"] != true {
		t.Fatalf("rd: %v", got)
	}

	// Cloud subnets start with the provider's five addresses reserved.
	cloud := data(c.must(201, "POST", "/subnets", gin.H{"routingDomainId": vpc, "cidr": "172.31.0.0/24", "name": "App", "environment": "production"}))
	u := data(c.must(200, "GET", "/subnets/"+cloud["id"].(string)+"/utilisation", nil))
	if u["reservedRangeAddresses"] != "3" || u["availableCandidates"] != "251" {
		t.Fatalf("aws reservation: %v", u)
	}
	if n := total(c.must(200, "GET", "/reserved-ranges?subnetId="+cloud["id"].(string), nil)); n != 2 {
		t.Fatalf("reserved ranges: %v", n)
	}
	code, out := c.call("DELETE", "/routing-domains/"+vpc, nil)
	if code != 409 || errorCode(out) != "RELATED_RECORD" {
		t.Fatalf("delete used domain: %d %v", code, out)
	}
	code, out = c.call("DELETE", "/organisations/"+org, nil)
	if code != 409 {
		t.Fatalf("delete used org: %d %v", code, out)
	}

	// Edits are version-checked and can map every relationship.
	subnet := func(cidr string, extra gin.H) gin.H {
		b := gin.H{"routingDomainId": rd, "cidr": cidr, "name": "Net " + cidr, "environment": "testing"}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}
	a := data(c.must(201, "POST", "/subnets", subnet("10.50.0.0/24", nil)))
	aid := a["id"].(string)
	bu := id(c.must(201, "POST", "/business-units", gin.H{"organisationId": org, "name": "Retail", "code": "RTL"}))
	edited := data(c.must(200, "PATCH", "/subnets/"+aid, gin.H{"name": "Renamed", "businessUnitId": bu, "sensitivity": "sensitive"}, "If-Match", "1"))
	if edited["name"] != "Renamed" || edited["businessUnit"].(map[string]any)["name"] != "Retail" || edited["version"].(float64) != 2 {
		t.Fatalf("edit: %v", edited)
	}
	c.must(412, "PATCH", "/subnets/"+aid, gin.H{"name": "Stale"}, "If-Match", "1")
	c.must(422, "PATCH", "/subnets/"+aid, gin.H{"sensitivity": "secret"}, "If-Match", "2")

	// Exceptions allow an overlap, can't lapse while it remains, and expired ones can't be applied.
	ex := id(c.must(201, "POST", "/overlap-exceptions", gin.H{"reason": "Migration window", "expiresAt": "2099-01-31"}))
	expired := id(c.must(201, "POST", "/overlap-exceptions", gin.H{"reason": "Old", "expiresAt": "2001-01-01"}))
	c.must(422, "POST", "/overlap-exceptions", gin.H{"reason": "Bad date", "expiresAt": "soon"})
	c.must(409, "POST", "/subnets", subnet("10.50.0.0/25", nil))
	c.must(422, "POST", "/subnets", subnet("10.50.0.0/25", gin.H{"overlapExceptionId": expired}))
	inner := data(c.must(201, "POST", "/subnets", subnet("10.50.0.0/25", gin.H{"overlapExceptionId": ex})))
	if inner["overlapExceptionId"] != ex {
		t.Fatalf("exception not returned: %v", inner)
	}
	code, out = c.call("PATCH", "/subnets/"+inner["id"].(string), gin.H{"overlapExceptionId": ""}, "If-Match", "1")
	if code != 409 || errorCode(out) != "SUBNET_OVERLAP" {
		t.Fatalf("removing a needed exception: %d %v", code, out)
	}
	if got := data(c.must(200, "GET", "/overlap-exceptions/"+ex, nil)); got["subnetCount"].(float64) != 1 || got["expired"] != false {
		t.Fatalf("exception: %v", got)
	}

	// Bulk changes are all or nothing.
	b := id(c.must(201, "POST", "/subnets", subnet("10.51.0.0/24", nil)))
	d := id(c.must(201, "POST", "/subnets", subnet("10.52.0.0/24", nil)))
	if got := data(c.must(200, "POST", "/subnets/bulk", gin.H{"ids": []string{aid, b}, "action": "update", "changes": gin.H{"environment": "uat", "discoveryEnabled": true}})); got["affected"].(float64) != 2 {
		t.Fatalf("bulk update: %v", got)
	}
	if got := data(c.must(200, "GET", "/subnets/"+b, nil)); got["environment"] != "uat" || got["discoveryEnabled"] != true {
		t.Fatalf("bulk result: %v", got)
	}
	c.must(204, "POST", "/subnets/"+d+"/archive", nil)
	code, out = c.call("POST", "/subnets/bulk", gin.H{"ids": []string{b, d}, "action": "update", "changes": gin.H{"environment": "production"}})
	if code != 409 {
		t.Fatalf("bulk with an archived subnet: %d %v", code, out)
	}
	if got := data(c.must(200, "GET", "/subnets/"+b, nil)); got["environment"] != "uat" {
		t.Fatal("a failed bulk change was partly applied")
	}
	c.must(422, "POST", "/subnets/bulk", gin.H{"ids": []string{b}, "action": "update", "changes": gin.H{"name": "Same"}})
	c.must(200, "POST", "/subnets/bulk", gin.H{"ids": []string{d}, "action": "delete"})
	c.must(404, "GET", "/subnets/"+d, nil)

	// IP records: bulk ownership changes, and filters by VLAN context.
	ip1 := id(c.must(201, "POST", "/ip-addresses", gin.H{"routingDomainId": rd, "address": "10.51.0.10", "hostname": "web01"}))
	ip2 := id(c.must(201, "POST", "/ip-addresses", gin.H{"routingDomainId": rd, "address": "10.51.0.11", "allocationState": "allocated"}))
	service := id(c.must(201, "POST", "/business-services", gin.H{"name": "Payments Portal"}))
	c.must(422, "POST", "/ip-addresses/bulk", gin.H{"ids": []string{ip1, ip2}, "changes": gin.H{"hostname": "same"}})
	c.must(422, "POST", "/ip-addresses/bulk", gin.H{"ids": []string{ip1, ip2}, "changes": gin.H{"allocationState": "available_candidate"}})
	c.must(200, "POST", "/ip-addresses/bulk", gin.H{"ids": []string{ip1, ip2}, "changes": gin.H{"businessServiceId": service}})
	if n := total(c.must(200, "GET", "/ip-addresses?businessServiceId="+service+"&family=4", nil)); n != 2 {
		t.Fatalf("bulk ip: %v", n)
	}

	// Global search: by address, CIDR, name and VLAN number.
	c.must(201, "POST", "/vlans", gin.H{"vlanId": 731, "name": "Core"})
	kinds := func(q string) map[string]int {
		out := map[string]int{}
		for _, r := range rows(c.must(200, "GET", "/search?q="+q, nil)) {
			out[r.(map[string]any)["type"].(string)]++
		}
		return out
	}
	if k := kinds("10.51.0.10"); k["subnet"] != 1 || k["ip_address"] != 1 {
		t.Fatalf("address search: %v", k)
	}
	if k := kinds("10.50.0.0/16"); k["subnet"] != 2 {
		t.Fatalf("cidr search: %v", k)
	}
	if k := kinds("web01"); k["ip_address"] != 1 {
		t.Fatalf("hostname search: %v", k)
	}
	if k := kinds("731"); k["vlan"] != 1 {
		t.Fatalf("vlan search: %v", k)
	}
	c.must(422, "GET", "/search?q=x", nil)

	// Saved filters belong to the user.
	f := id(c.must(201, "POST", "/saved-filters", gin.H{"page": "subnets", "name": "UAT", "filters": gin.H{"environment": "uat"}}))
	c.must(409, "POST", "/saved-filters", gin.H{"page": "subnets", "name": "UAT", "filters": gin.H{"environment": "uat"}})
	if n := total(c.must(200, "GET", "/saved-filters?page=subnets", nil)); n != 1 {
		t.Fatalf("saved filters: %v", n)
	}
	c.must(204, "DELETE", "/saved-filters/"+f, nil)
	c.must(404, "DELETE", "/saved-filters/"+f, nil)

	// Read-only DNS and DHCP inventory, and the owner picker.
	c.must(200, "GET", "/dns-records?ipAddressId="+ip1, nil)
	scope := id(c.must(201, "POST", "/dhcp-scopes", gin.H{"subnetId": b, "rangeStart": "10.51.0.100", "rangeEnd": "10.51.0.150"}))
	if n := total(c.must(200, "GET", "/dhcp-scopes/"+scope+"/reservations", nil)); n != 0 {
		t.Fatal(n)
	}
	if n := total(c.must(200, "GET", "/users/lookup?q=it@", nil)); n != 1 {
		t.Fatalf("lookup: %v", n)
	}
	c.must(404, "POST", "/dns-records", gin.H{"name": "x"})
}
