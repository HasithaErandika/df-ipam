package server

import (
	"context"
	"testing"

	"dfipam/api/internal/auth"

	"github.com/gin-gonic/gin"
)

func TestPhaseOneCompletion(t *testing.T) {
	p, tx := setupPeople(t)
	admin := p.user("p1admin@example.com", auth.SuperAdminRole)
	other := p.user("p1other@example.com", "Infrastructure Engineer")
	c := p.as(admin)

	// Organisations and routing domains can be edited, and deleted only when empty.
	org := id(c.must(201, "POST", "/organisations", gin.H{"name": "P1 Org", "code": "p1"}))
	if got := data(c.must(200, "PATCH", "/organisations/"+org, gin.H{"name": "P1 Organisation", "code": "p1x"})); got["code"] != "P1X" {
		t.Fatalf("org update: %v", got)
	}
	vpc := id(c.must(201, "POST", "/routing-domains", gin.H{"organisationId": org, "name": "Prod VPC", "kind": "vpc"}))
	if got := data(c.must(200, "PATCH", "/routing-domains/"+vpc, gin.H{"name": "Prod VPC 1", "routeDistinguisher": "65000:1"})); got["name"] != "Prod VPC 1" || got["kind"] != "vpc" {
		t.Fatalf("rd update: %v", got)
	}

	// A subnet in an AWS VPC gets AWS's reserved addresses recorded.
	sub := data(c.must(201, "POST", "/subnets", gin.H{"routingDomainId": vpc, "cidr": "172.31.0.0/24", "name": "Public A", "environment": "production"}))
	if n := count(t, tx, `SELECT count(*) FROM subnet_reserved_ranges WHERE subnet_id = $1::uuid AND provider = 'aws'`, sub["id"]); n != 2 {
		t.Fatalf("aws reserved ranges: %d", n)
	}
	if sub["reservedRangeAddresses"] != "3" { // .1 to .3: network and broadcast are never usable anyway
		t.Fatalf("reserved addresses counted: %v", sub["reservedRangeAddresses"])
	}
	c.must(409, "DELETE", "/routing-domains/"+vpc, nil)
	c.must(409, "DELETE", "/organisations/"+org, nil)

	// Bulk create: a dry run saves nothing, one bad row saves nothing, a clean batch saves all.
	rows := []gin.H{{"cidr": "172.31.1.0/24", "name": "B"}, {"cidr": "172.31.2.0/24", "name": "C"}}
	dry := data(c.must(200, "POST", "/subnets/bulk", gin.H{"routingDomainId": vpc, "environment": "testing", "dryRun": true, "subnets": rows}))
	if dry["created"] != false || dry["valid"] != float64(2) {
		t.Fatalf("dry run: %v", dry)
	}
	bad := data(c.must(200, "POST", "/subnets/bulk", gin.H{"routingDomainId": vpc, "environment": "testing",
		"subnets": append(rows, gin.H{"cidr": "172.31.1.128/25", "name": "overlaps B"}, gin.H{"cidr": "nonsense", "name": "x"})}))
	results := bad["results"].([]any)
	if bad["created"] != false || bad["invalid"] != float64(2) || results[2].(map[string]any)["code"] != "SUBNET_OVERLAP" {
		t.Fatalf("bad batch: %v", bad)
	}
	if n := count(t, tx, `SELECT count(*) FROM subnets WHERE routing_domain_id = $1::uuid`, vpc); n != 1 {
		t.Fatalf("rejected batch left %d subnets", n)
	}
	done := data(c.must(201, "POST", "/subnets/bulk", gin.H{"routingDomainId": vpc, "environment": "testing", "subnets": rows}))
	var ids []string
	for _, r := range done["results"].([]any) {
		ids = append(ids, r.(map[string]any)["id"].(string))
	}
	c.must(200, "POST", "/subnets/bulk-archive", gin.H{"ids": ids})
	if n := count(t, tx, `SELECT count(*) FROM subnets WHERE id = ANY($1::uuid[]) AND archived_at IS NOT NULL`, ids); n != 2 {
		t.Fatalf("bulk archive: %d", n)
	}
	p.as(other).must(403, "POST", "/subnets/bulk", gin.H{"routingDomainId": vpc, "subnets": rows})

	// DNS records are read-only inventory.
	ctx := context.Background()
	if _, err := tx.Exec(ctx, `INSERT INTO dns_records (routing_domain_id, name, record_type, value) VALUES ($1::uuid, 'app01.df.lk', 'A', '172.31.0.10')`, vpc); err != nil {
		t.Fatal(err)
	}
	dns := p.as(admin).must(200, "GET", "/dns-records?q=app01", nil)
	if dns["meta"].(map[string]any)["total"] != float64(1) {
		t.Fatalf("dns list: %v", dns)
	}
	c = p.as(admin)
	c.must(404, "POST", "/dns-records", gin.H{"name": "x", "recordType": "A", "value": "1.1.1.1"})

	// Search: an address finds the subnet holding it; a name finds the subnet.
	if _, err := tx.Exec(ctx, `INSERT INTO ip_addresses (routing_domain_id, subnet_id, address, hostname) VALUES ($1::uuid, $2::uuid, '172.31.0.10', 'app01')`, vpc, sub["id"]); err != nil {
		t.Fatal(err)
	}
	found := data(c.must(200, "GET", "/search?q=172.31.0.10", nil))
	if len(found["subnets"].([]any)) != 1 || len(found["ipAddresses"].([]any)) != 1 {
		t.Fatalf("search by address: %v", found)
	}
	if byName := data(c.must(200, "GET", "/search?q=app0", nil)); len(byName["ipAddresses"].([]any)) != 1 {
		t.Fatalf("search by hostname: %v", byName)
	}
	c.must(422, "GET", "/search?q=x", nil)

	// The device view gathers inventory and read-only evidence for an address.
	var ip, device, iface, scope string
	tx.QueryRow(ctx, `SELECT id::text FROM ip_addresses WHERE address = '172.31.0.10' AND routing_domain_id = $1::uuid`, vpc).Scan(&ip)
	device = id(c.must(201, "POST", "/devices", gin.H{"name": "app01", "vendor": "Dell", "model": "R650", "deviceType": "physical_server", "osVersion": "RHEL 9.4"}))
	iface = id(c.must(201, "POST", "/interfaces", gin.H{"deviceId": device, "name": "eno1", "mac": "02:42:ac:1f:00:0a"}))
	c.must(200, "PATCH", "/ip-addresses/"+ip, gin.H{"deviceId": device})
	if _, err := tx.Exec(ctx, `UPDATE ip_addresses SET interface_id = $2::uuid WHERE id = $1::uuid`, ip, iface); err != nil {
		t.Fatal(err)
	}
	scope = id(c.must(201, "POST", "/dhcp-scopes", gin.H{"subnetId": sub["id"], "name": "pool", "rangeStart": "172.31.0.100", "rangeEnd": "172.31.0.200"}))
	if _, err := tx.Exec(ctx, `INSERT INTO dhcp_reservations (scope_id, address, hostname) VALUES ($1::uuid, '172.31.0.10', 'app01')`, scope); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO discovery_observations (source_id, routing_domain_id, address, payload, observed_at)
		SELECT id, $1::uuid, '172.31.0.10', '{}', now() FROM observation_sources WHERE name = 'DNS'`, vpc); err != nil {
		t.Fatal(err)
	}
	info := data(c.must(200, "GET", "/ip-addresses/"+ip+"/device", nil))
	dev := info["device"].(map[string]any)
	if dev["name"] != "app01" || dev["osVersion"] != "RHEL 9.4" || info["interface"].(map[string]any)["name"] != "eno1" {
		t.Fatalf("device view: %v", info)
	}
	if info["mac"].(map[string]any)["locallyAdministered"] != true || len(info["dnsRecords"].([]any)) != 1 ||
		len(info["dhcpReservations"].([]any)) != 1 || info["hasEvidence"] != true {
		t.Fatalf("device view sources: %v", info)
	}

	// Saved filters belong to their owner.
	saved := data(c.must(201, "POST", "/saved-filters", gin.H{"page": "subnets", "name": "Prod", "filters": gin.H{"environment": "production"}}))
	if list := c.must(200, "GET", "/saved-filters?page=subnets", nil)["data"].([]any); len(list) != 1 {
		t.Fatalf("saved filters: %v", list)
	}
	p.as(other).must(404, "DELETE", "/saved-filters/"+saved["id"].(string), nil)
	p.as(admin).must(204, "DELETE", "/saved-filters/"+saved["id"].(string), nil)

	// An empty organisation and routing domain can be deleted.
	empty := id(p.as(admin).must(201, "POST", "/organisations", gin.H{"name": "Empty", "code": "empty"}))
	p.as(admin).must(204, "DELETE", "/organisations/"+empty, nil)
}
