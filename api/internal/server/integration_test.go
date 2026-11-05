package server

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestFullInventoryFlow(t *testing.T) {
	c := setup(t)
	org := id(c.must(201, "POST", "/organisations", gin.H{"name": "IT Org", "code": "it"}))
	domains := c.must(200, "GET", "/routing-domains?pageSize=200", nil)["data"].([]any)
	var rd string
	for _, d := range domains {
		m := d.(map[string]any)
		if m["organisationId"] == org {
			rd = m["id"].(string)
		}
	}
	rd2 := id(c.must(201, "POST", "/routing-domains", gin.H{"organisationId": org, "name": "VRF-B", "kind": "vrf"}))

	site := id(c.must(201, "POST", "/sites", gin.H{"organisationId": org, "name": "Colombo DC", "code": "CMB", "siteType": "data_centre"}))
	site2 := id(c.must(201, "POST", "/sites", gin.H{"organisationId": org, "name": "Kandy DR", "code": "KDY"}))
	vlan := id(c.must(201, "POST", "/vlans", gin.H{"vlanId": 120, "name": "App", "siteId": site}))
	service := id(c.must(201, "POST", "/business-services", gin.H{"name": "IT Lending"}))
	block := id(c.must(201, "POST", "/network-blocks", gin.H{"routingDomainId": rd, "cidr": "10.20.0.0/16", "name": "Campus"}))
	c.must(422, "POST", "/network-blocks", gin.H{"routingDomainId": rd, "cidr": "10.20.0.5/16", "name": "Bad"})
	c.must(409, "POST", "/sites", gin.H{"organisationId": org, "name": "Dup", "code": "CMB"})

	subnet := func(cidr string, extra gin.H) gin.H {
		b := gin.H{"routingDomainId": rd, "cidr": cidr, "name": "Net " + cidr, "environment": "production"}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}
	// Mapping is stored, named in responses, and checked for consistency.
	created := data(c.must(201, "POST", "/subnets", subnet("10.20.30.0/24", gin.H{"gateway": "10.20.30.1", "siteId": site, "vlanId": vlan, "businessServiceId": service, "networkBlockId": block})))
	sid := created["id"].(string)
	if created["site"].(map[string]any)["name"] != "Colombo DC" || created["vlan"].(map[string]any)["name"] != "120 App" {
		t.Fatalf("mapping not returned: %v", created)
	}
	c.must(422, "POST", "/subnets", subnet("10.20.31.0/24", gin.H{"siteId": site2, "vlanId": vlan}))
	c.must(422, "POST", "/subnets", subnet("10.99.0.0/24", gin.H{"networkBlockId": block}))
	c.must(409, "POST", "/subnets", subnet("10.20.30.128/25", nil))
	c.must(201, "POST", "/subnets", gin.H{"routingDomainId": rd2, "cidr": "10.20.30.0/24", "name": "Same in VRF", "environment": "testing"})

	list := c.must(200, "GET", "/subnets?siteId="+site+"&sort=-utilisation", nil)
	if list["meta"].(map[string]any)["total"].(float64) != 1 {
		t.Fatalf("site filter: %v", list["meta"])
	}

	// Pagination: pages don't overlap, and metadata is correct on every page.
	pageIDs := map[string]bool{}
	c.must(201, "POST", "/subnets", subnet("10.20.40.0/24", nil))
	c.must(201, "POST", "/subnets", subnet("10.20.41.0/24", nil))
	for page, want := range []int{2, 1} {
		res := c.must(200, "GET", fmt.Sprintf("/subnets?routingDomainId=%s&pageSize=2&page=%d", rd, page+1), nil)
		rows := res["data"].([]any)
		meta := res["meta"].(map[string]any)
		if len(rows) != want || meta["total"].(float64) != 3 || meta["totalPages"].(float64) != 2 || meta["hasNext"].(bool) != (page == 0) || meta["hasPrev"].(bool) != (page == 1) {
			t.Fatalf("page %d: %d rows, %v", page+1, len(rows), meta)
		}
		for _, row := range rows {
			pageIDs[row.(map[string]any)["id"].(string)] = true
		}
	}
	if len(pageIDs) != 3 {
		t.Fatalf("pages overlapped: %v", pageIDs)
	}
	past := c.must(200, "GET", "/subnets?routingDomainId="+rd+"&pageSize=2&page=9", nil)
	if rows := past["data"].([]any); len(rows) != 0 || past["meta"].(map[string]any)["totalPages"].(float64) != 2 {
		t.Fatalf("past the end: %v", past)
	}

	// Records: parent subnet is found, network and broadcast are refused.
	addr := func(address string, extra gin.H) gin.H {
		b := gin.H{"routingDomainId": rd, "address": address}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}
	ip := data(c.must(201, "POST", "/ip-addresses", addr("10.20.30.10", gin.H{"hostname": "app01", "mac": "aa:bb:cc:dd:ee:01", "allocationState": "allocated"})))
	if ip["subnetName"] != "Net 10.20.30.0/24" || ip["allocationState"] != "allocated" {
		t.Fatalf("ip: %v", ip)
	}
	c.must(201, "POST", "/ip-addresses", addr("10.20.30.11", gin.H{"allocationState": "reserved"}))
	c.must(201, "POST", "/ip-addresses", addr("10.20.30.12", gin.H{"allocationState": "quarantined", "mac": "aa:bb:cc:dd:ee:01"}))
	c.must(409, "POST", "/ip-addresses", addr("10.20.30.10", nil))
	c.must(422, "POST", "/ip-addresses", addr("10.20.30.0", nil))
	c.must(422, "POST", "/ip-addresses", addr("10.20.30.255", nil))
	c.must(422, "POST", "/ip-addresses", addr("192.0.2.1", nil))

	c.must(201, "POST", "/reserved-ranges", gin.H{"subnetId": sid, "rangeStart": "10.20.30.1", "rangeEnd": "10.20.30.4", "provider": "aws"})
	c.must(201, "POST", "/dhcp-scopes", gin.H{"subnetId": sid, "rangeStart": "10.20.30.100", "rangeEnd": "10.20.30.199"})

	// 254 usable: 3 records + 4 reserved = 7 used. DHCP also blocks 100 candidates.
	u := data(c.must(200, "GET", "/subnets/"+sid+"/utilisation", nil))
	if u["utilisationPct"] != "2.76" || u["availableCandidates"] != "147" || u["dhcpPoolAddresses"] != "100" || u["reservedRangeAddresses"] != "4" {
		t.Fatalf("utilisation: %v", u)
	}
	free := data(c.must(200, "GET", "/subnets/"+sid+"/free-ranges", nil))
	if free["totalFree"] != "147" {
		t.Fatalf("free: %v", free)
	}
	grid := c.must(200, "GET", "/subnets/"+sid+"/addresses?offset=0&limit=16", nil)
	cells := grid["data"].([]any)
	kinds := map[string]string{}
	for _, cell := range cells {
		m := cell.(map[string]any)
		kinds[m["address"].(string)] = m["kind"].(string)
	}
	if kinds["10.20.30.0"] != "network" || kinds["10.20.30.2"] != "reserved_range" || kinds["10.20.30.10"] != "record" || kinds["10.20.30.13"] != "candidate" {
		t.Fatalf("grid: %v", kinds)
	}

	checkPlanningTools(t, c, block)

	// Releasing needs a reason, and the allocation history follows the record.
	ipID := ip["id"].(string)
	c.must(422, "PATCH", "/ip-addresses/"+ipID, gin.H{"allocationState": "available_candidate"})
	released := data(c.must(200, "PATCH", "/ip-addresses/"+ipID, gin.H{"allocationState": "available_candidate", "reason": "Decommissioned", "hostname": "old"}))
	if released["allocationState"] != "available_candidate" || released["hostname"] != "old" {
		t.Fatalf("release: %v", released)
	}
	c.must(200, "PATCH", "/ip-addresses/"+ipID, gin.H{"allocationState": "allocated"})
	if hist := c.must(200, "GET", "/ip-addresses/"+ipID+"/history", nil)["data"].([]any); len(hist) != 3 {
		t.Fatalf("history: %v", hist)
	}
	c.must(200, "GET", "/ip-addresses/"+ipID+"/evidence", nil)
	if m := c.must(200, "GET", "/ip-addresses?q=old&allocationState=allocated&subnetId="+sid, nil)["meta"].(map[string]any); m["total"].(float64) != 1 {
		t.Fatalf("ip search: %v", m)
	}

	// Findings: detected, stable across runs, and dismissals stick.
	run := data(c.must(200, "POST", "/reconciliation/run", nil))
	if run["detected"].(float64) == 0 {
		t.Fatal("expected findings")
	}
	results := c.must(200, "GET", "/reconciliation/results?pageSize=200", nil)
	first := results["meta"].(map[string]any)["total"].(float64)
	found := map[string]map[string]any{}
	for _, r := range results["data"].([]any) {
		m := r.(map[string]any)
		found[m["classification"].(string)] = m
	}
	for _, want := range []string{"ownership_missing", "duplicate_mac"} {
		if found[want] == nil {
			t.Fatalf("missing %s: %v", want, found)
		}
	}
	if strings.Contains(fmt.Sprint(found), "subnet_overlap") {
		t.Fatal("same range in separate routing domains was flagged")
	}
	c.must(200, "POST", "/reconciliation/run", nil)
	again := c.must(200, "GET", "/reconciliation/results?pageSize=200", nil)["meta"].(map[string]any)["total"].(float64)
	if again != first {
		t.Fatalf("duplicates: %v then %v", first, again)
	}
	mac := found["duplicate_mac"]["id"].(string)
	c.must(204, "POST", "/reconciliation/results/"+mac+"/review", gin.H{"status": "dismissed", "note": "HA pair"})
	c.must(200, "POST", "/reconciliation/run", nil)
	if got := c.must(200, "GET", "/reconciliation/results?classification=duplicate_mac", nil)["meta"].(map[string]any)["total"].(float64); got != 0 {
		t.Fatal("dismissed finding came back")
	}
	if s := c.must(200, "GET", "/reconciliation/summary", nil)["data"].([]any); len(s) == 0 {
		t.Fatal("empty summary")
	}
	if ip := data(c.must(200, "GET", "/ip-addresses/"+ipID, nil)); ip["allocationState"] != "allocated" {
		t.Fatal("a run changed inventory")
	}

	sum := data(c.must(200, "GET", "/dashboard/summary", nil))
	if sum["openFindings"].(float64) == 0 || sum["quarantined"].(float64) < 1 {
		t.Fatalf("dashboard: %v", sum)
	}
	if logs := c.must(200, "GET", "/audit-logs?objectType=ip_address", nil)["meta"].(map[string]any)["total"].(float64); logs < 3 {
		t.Fatalf("audit: %v", logs)
	}

	// Editing and deleting reference data.
	if got := data(c.must(200, "PATCH", "/sites/"+site2, gin.H{"name": "Kandy"}))["name"]; got != "Kandy" {
		t.Fatal(got)
	}
	c.must(409, "DELETE", "/sites/"+site, nil)
	c.must(204, "DELETE", "/sites/"+site2, nil)
	c.must(204, "POST", "/subnets/"+sid+"/archive", nil)
}
