package server

import (
	"context"
	"testing"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/integrations"
	"dfipam/api/internal/platform/safehttp"

	"github.com/gin-gonic/gin"
)

func fullSyncResult() *integrations.Result {
	now := time.Now().Add(-time.Minute)
	return &integrations.Result{
		Devices: []integrations.Device{{ExternalID: "fw9", Name: "fw9", DeviceType: "firewall", ManagementIP: "10.66.0.1",
			VDOMs:      []integrations.VDOM{{Name: "root", Context: "vdom:root"}},
			Interfaces: []integrations.Interface{{Name: "port1", MAC: "aa:bb:cc:dd:ee:01", Type: "physical", VDOM: "root", Context: "vdom:root"}}}},
		Accounts: []integrations.CloudAccount{{Provider: "aws", ExternalID: "acct", Name: "Account"}},
		Networks: []integrations.CloudNetwork{
			{AccountExternalID: "acct", ExternalID: "vpc-9", Name: "VPC", Region: "ap-south-1", AddressSpaces: []string{"10.67.0.1/16", "bad"}},
			{AccountExternalID: "nobody", ExternalID: "vpc-0", Name: "Orphan"},
		},
		AddressObjects: []integrations.AddressObject{
			{DeviceExternalID: "fw9", VDOM: "root", Name: "a", Type: "ipmask", Value: "10.66.1.0/24", Network: "10.66.1.0/24"},
			{DeviceExternalID: "fw9", VDOM: "root", Name: "b", Type: "fqdn", Value: "x.example"},
			{DeviceExternalID: "nobody", VDOM: "root", Name: "c"},
		},
		AddressGroups: []integrations.AddressGroup{
			{DeviceExternalID: "fw9", VDOM: "root", Name: "g", Members: []string{"a", "b", "nested"}},
			{DeviceExternalID: "nobody", VDOM: "root", Name: "h"},
		},
		Routes: []integrations.Route{
			{Context: "vdom:root", DeviceExternalID: "fw9", VDOM: "root", Destination: "10.66.2.5/24", NextHop: "10.66.0.254", Type: "static"},
			{Context: "vdom:root", DeviceExternalID: "fw9", VDOM: "root", Destination: "not a network"},
			{Context: "vdom:root", DeviceExternalID: "nobody", Destination: "10.66.3.0/24"},
		},
		Peerings: []integrations.Peering{{ContextA: "vdom:root", ContextB: "vpc-9", Kind: "peering"}, {ContextA: "vdom:root", ContextB: "vdom:root", Kind: "peering"}},
		DHCPScopes: []integrations.DHCPScope{
			{Context: "vdom:root", DeviceExternalID: "fw9", Name: "first", RangeStart: "10.66.1.10", RangeEnd: "10.66.1.20"},
			{Context: "vdom:root", DeviceExternalID: "fw9", Name: "listed twice", RangeStart: "10.66.1.10", RangeEnd: "10.66.1.20"},
			{Context: "vdom:root", Name: "no subnet", RangeStart: "10.99.1.10", RangeEnd: "10.99.1.20"},
		},
		Observations: []integrations.Observation{{Source: "FortiGate", Context: "vdom:root", Address: "10.66.1.5", Alive: true, ObservedAt: now}},
		Warnings:     []integrations.Warning{{Code: "ONE", Message: "first"}, {Code: "TWO", Message: "second", Context: "vdom:root"}},
	}
}

// A sync stores every kind of evidence, and running it twice changes no counts.
func TestSyncStoresEveryKindOfEvidence(t *testing.T) {
	p, tx := setupPeople(t)
	ctx := context.Background()
	c := p.as(p.user("sync@example.com", auth.SuperAdminRole))
	org := id(c.must(201, "POST", "/organisations", gin.H{"name": "Sync Org", "code": "syncorg"}))
	rd := id(c.must(201, "POST", "/routing-domains", gin.H{"organisationId": org, "name": "Sync VRF", "kind": "vrf"}))
	cloud := id(c.must(201, "POST", "/routing-domains", gin.H{"organisationId": org, "name": "Sync VPC", "kind": "vrf"}))
	c.must(201, "POST", "/subnets", gin.H{"routingDomainId": rd, "cidr": "10.66.1.0/24", "name": "Sync LAN", "environment": "production"})
	conn := id(c.must(201, "POST", "/integrations/connections", gin.H{"kind": "fortigate", "name": "full", "baseUrl": "https://fw.test.example",
		"credentials": gin.H{"apiToken": "t"}, "routingDomains": gin.H{"vdom:root": rd, "vpc-9": cloud}}))
	svc := integrations.NewService(tx, testKeys(t), testPolicy).WithBuilder(func(integrations.Connection, map[string]string, safehttp.Policy) (integrations.Connector, error) {
		return fixedConnector{result: fullSyncResult()}, nil
	})
	counts := func(sql string, want int) {
		t.Helper()
		if n := count(t, tx, sql, conn); n != want {
			t.Fatalf("%s: got %d want %d", sql, n, want)
		}
	}
	for run := range 2 {
		c.must(202, "POST", "/integrations/connections/"+conn+"/sync", nil)
		if res, err := svc.RunSyncs(ctx); err != nil || res.Completed != 1 {
			t.Fatalf("run %d: %+v %v", run, res, err)
		}
		const device = `(SELECT id FROM devices WHERE external_id = $1 || ':fw9')`
		counts(`SELECT count(*) FROM vdoms WHERE device_id = `+device, 1)
		counts(`SELECT count(*) FROM device_interfaces i JOIN vdoms v ON v.id = i.vdom_id WHERE i.device_id = `+device+` AND i.name = 'port1' AND i.mac = 'aa:bb:cc:dd:ee:01'`, 1)
		counts(`SELECT count(*) FROM external_resource_mappings WHERE connection_id = $1::uuid`, 3)
		counts(`SELECT count(*) FROM cloud_accounts WHERE connection_id = $1::uuid`, 1)
		counts(`SELECT count(*) FROM cloud_networks n JOIN cloud_accounts a ON a.id = n.cloud_account_id WHERE a.connection_id = $1::uuid AND n.address_spaces = ARRAY['10.67.0.0/16'::cidr]`, 1)
		counts(`SELECT count(*) FROM firewall_address_objects WHERE device_id = `+device, 2)
		counts(`SELECT count(*) FROM firewall_address_groups WHERE device_id = `+device, 1)
		counts(`SELECT count(*) FROM firewall_address_group_members m JOIN firewall_address_groups g ON g.id = m.group_id WHERE g.device_id = `+device, 2)
		counts(`SELECT count(*) FROM network_routes WHERE device_id = `+device+` AND destination = '10.66.2.0/24' AND next_hop = '10.66.0.254'`, 1)
		counts(`SELECT count(*) FROM network_routes WHERE device_id = `+device, 1)
		counts(`SELECT count(*) FROM dhcp_scopes d JOIN subnets s ON s.id = d.subnet_id JOIN devices v ON v.id = d.server_device_id
			WHERE v.external_id = $1 || ':fw9' AND d.name = 'listed twice'`, 1)
		if n := count(t, tx, `SELECT count(*) FROM routing_domain_peerings WHERE $1::uuid IN (domain_a_id, domain_b_id)`, rd); n != 1 {
			t.Fatalf("peerings: %d", n)
		}
		counts(`SELECT count(*) FROM integration_errors e JOIN integration_sync_jobs j ON j.id = e.sync_job_id
			WHERE j.connection_id = $1::uuid AND e.code = 'TWO' AND e.context->>'context' = 'vdom:root'`, run+1)
	}
	if got := text(t, tx, `SELECT counts::text FROM integration_sync_jobs WHERE connection_id = $1::uuid ORDER BY created_at DESC, id LIMIT 1`, conn); got == "" {
		t.Fatal("no counts")
	}
	var skipped, routes, scopes int
	if err := tx.QueryRow(ctx, `SELECT (counts->>'skipped')::int, (counts->>'routes')::int, (counts->>'dhcpScopes')::int FROM integration_sync_jobs WHERE connection_id = $1::uuid LIMIT 1`, conn).
		Scan(&skipped, &routes, &scopes); err != nil {
		t.Fatal(err)
	}
	if skipped != 6 || routes != 1 || scopes != 2 {
		t.Fatalf("skipped %d routes %d scopes %d", skipped, routes, scopes)
	}
}
