package server

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"dfipam/api/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"
)

func workbook(t *testing.T, sheets map[string][][]any) string {
	t.Helper()
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	for name, rows := range sheets {
		if _, err := f.NewSheet(name); err != nil {
			t.Fatal(err)
		}
		for i := range rows {
			if err := f.SetSheetRow(name, fmt.Sprintf("A%d", i+1), &rows[i]); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := f.DeleteSheet("Sheet1"); err != nil {
		t.Fatal(err)
	}
	b, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

func network(prefix string) map[string][][]any {
	return map[string][][]any{
		"Organisations":   {{"name *", "code *"}, {prefix + " Finance", prefix}},
		"Routing domains": {{"organisation *", "name *", "kind *"}, {prefix, "Core", "vrf"}},
		"Sites":           {{"organisation *", "name *", "code *", "site_type"}, {prefix, "Head Office", "HQ", "data_centre"}},
		"Devices":         {{"name *", "device_type", "site"}, {prefix + "-fw", "firewall", "HQ"}},
		"VLANs":           {{"vlan_id *", "name", "site"}, {"210", "Users", "HQ"}},
		"Network blocks":  {{"routing_domain *", "cidr *", "name *"}, {"Core", "10.210.0.0/16", "Wide"}, {"Core", "10.210.1.0/24", "Narrow"}},
		"Subnets": {{"routing_domain *", "cidr *", "name *", "environment *", "gateway", "site", "vlan", "network_block"},
			{"Core", "10.210.1.0/25", "Users", "production", "10.210.1.1", "HQ", "210", "10.210.1.0/24"},
			{"Core", "10.210.1.0/26", "Inside users", "production"},
			{"Core", "10.210.2.0/24", "Bad gateway", "production", "10.99.0.1"},
			{"Missing", "10.210.3.0/24", "No domain", "production"}},
		"IP addresses": {{"routing_domain *", "address *", "hostname", "device", "allocation_state"},
			{"Core", "10.210.1.10", "printer", prefix + "-fw", "allocated"}, {"Core", "10.77.0.1", "outside"}},
		"Reserved ranges": {{"routing_domain *", "subnet *", "range_start *", "range_end *"}, {"Core", "10.210.1.0/25", "10.210.1.2", "10.210.1.9"}},
	}
}

func TestTemplateImportFromUploadToReversal(t *testing.T) {
	p, tx := setupPeople(t)
	uploader := p.user("tpl-up@example.com", auth.SuperAdminRole)
	approver := p.user("tpl-ok@example.com", auth.SuperAdminRole)
	upload := gin.H{"fileName": "network.xlsx", "format": "template", "content": workbook(t, network("TPL"))}
	batch := data(p.as(uploader).must(201, "POST", "/imports", upload))
	id := batch["id"].(string)
	s := batch["summary"].(map[string]any)
	if s["valid"] != float64(10) || s["invalid"] != float64(3) || s["overlap"] != float64(1) {
		t.Fatalf("summary: %v", s)
	}
	for row, want := range map[string]string{"Bad gateway": "outside 10.210.2.0/24", "10.210.3.0/24": "isn't in routing domains", "10.77.0.1": "No subnet"} {
		got := text(t, tx, `SELECT errors->>0 FROM import_records WHERE batch_id = $1::uuid AND raw_text LIKE '%' || $2 || '%'`, id, row)
		if !strings.Contains(got, want) {
			t.Fatalf("%s: %s", row, got)
		}
	}
	if n := count(t, tx, `SELECT count(*) FROM organisations WHERE code = 'TPL'`); n != 0 {
		t.Fatal("nothing may land before approval")
	}
	p.as(uploader).must(403, "POST", "/imports/"+id+"/approve", nil)
	p.as(approver).must(200, "POST", "/imports/"+id+"/approve", nil)

	rd := text(t, tx, `SELECT r.id::text FROM routing_domains r JOIN organisations o ON o.id = r.organisation_id WHERE o.code = 'TPL' AND r.name = 'Core'`)
	if n := count(t, tx, `SELECT count(*) FROM subnets s JOIN sites st ON st.id = s.site_id JOIN vlans v ON v.id = s.vlan_id
		WHERE s.routing_domain_id = $1::uuid AND s.cidr = '10.210.1.0/25' AND s.network_block_id IS NOT NULL`, rd); n != 1 {
		t.Fatal("the subnet should link to its site, VLAN and block")
	}
	if n := count(t, tx, `SELECT count(*) FROM ip_addresses i JOIN ip_allocations a ON a.ip_address_id = i.id WHERE i.routing_domain_id = $1::uuid AND i.device_id IS NOT NULL`, rd); n != 1 {
		t.Fatal("the address should be allocated and linked to its device")
	}
	if n := count(t, tx, `SELECT count(*) FROM network_blocks c JOIN network_blocks w ON w.id = c.parent_id WHERE c.cidr = '10.210.1.0/24' AND w.cidr = '10.210.0.0/16'`); n != 1 {
		t.Fatal("the narrow block should sit under the wide one")
	}
	if n := count(t, tx, `SELECT count(*) FROM audit_logs WHERE action IN ('site.create', 'device.create', 'vlan.create', 'reserved_range.create') AND object_id IN
		(SELECT created_object_id::text FROM import_records WHERE batch_id = $1::uuid)`, id); n != 4 {
		t.Fatalf("audit rows: %d", n)
	}

	// The same content again adds nothing: rows that exist are skipped.
	again := network("TPL")
	again["Teams"] = [][]any{{"name *"}, {"TPL Ops"}}
	repeat := data(p.as(uploader).must(201, "POST", "/imports", gin.H{"fileName": "again.xlsx", "format": "template", "content": workbook(t, again)}))
	if rs := repeat["summary"].(map[string]any); rs["valid"] != float64(1) || rs["duplicate"].(float64) < 10 {
		t.Fatalf("repeat: %v", rs)
	}
	p.as(uploader).must(200, "POST", "/imports/"+repeat["id"].(string)+"/discard", nil)
	p.as(approver).must(409, "POST", "/imports/"+repeat["id"].(string)+"/approve", nil)

	// Reversal stops if a record has changed, then succeeds once it is put right.
	move := func(by string) {
		t.Helper()
		if _, err := tx.Exec(t.Context(), `UPDATE import_batches SET committed_at = committed_at + $2::interval WHERE id = $1::uuid`, id, by); err != nil {
			t.Fatal(err)
		}
	}
	move("-1 hour")
	p.as(approver).must(409, "POST", "/imports/"+id+"/reverse", nil)
	move("1 hour")
	p.as(approver).must(200, "POST", "/imports/"+id+"/reverse", nil)
	for _, table := range []string{"ip_addresses", "subnets", "network_blocks"} {
		if n := count(t, tx, `SELECT count(*) FROM `+table+` WHERE routing_domain_id = $1::uuid`, rd); n != 0 {
			t.Fatalf("%s left behind: %d", table, n)
		}
	}
	if n := count(t, tx, `SELECT count(*) FROM organisations WHERE code = 'TPL'`); n != 0 {
		t.Fatal("the organisation should be gone")
	}
	// A reversed file can come back.
	p.as(uploader).must(201, "POST", "/imports", upload)
}

func TestTemplateRefusals(t *testing.T) {
	p, _ := setupPeople(t)
	c := p.as(p.user("tpl-no@example.com", auth.SuperAdminRole))
	for name, content := range map[string]string{
		"not a template":  "10.0.0.0/24,name=Plain",
		"unknown column":  "# dfipam-template: teams\nname,colour\nOps,blue\n",
		"missing column":  "# dfipam-template: sites\nname,code\nHQ,HQ\n",
		"only an example": "# dfipam-template: teams\nname\n# Ops\n",
	} {
		if status, body := c.call("POST", "/imports", gin.H{"fileName": "x.csv", "format": "template", "content": content}); status != 422 {
			t.Fatalf("%s: %d %v", name, status, body)
		}
	}
	list := c.must(200, "GET", "/imports/templates", nil)
	if n := len(list["data"].([]any)); n != 17 {
		t.Fatalf("templates: %d", n)
	}
	c.must(404, "GET", "/imports/templates/users", nil)
}

// A file can't make someone an allocator: the approver needs that right themselves.
func TestTemplateAllocationNeedsAllocator(t *testing.T) {
	p, _ := setupPeople(t)
	up := p.as(p.user("tpl-a@example.com", auth.SuperAdminRole))
	org := id(up.must(201, "POST", "/organisations", gin.H{"name": "Alloc Org", "code": "alc"}))
	rd := id(up.must(201, "POST", "/routing-domains", gin.H{"organisationId": org, "name": "Alloc VRF", "kind": "vrf"}))
	up.must(201, "POST", "/subnets", gin.H{"routingDomainId": rd, "cidr": "10.211.1.0/24", "name": "S", "environment": "production"})
	content := "# dfipam-template: ip_addresses\nrouting_domain,address,allocation_state\nAlloc VRF,10.211.1.5,allocated\n"
	batch := id(up.must(201, "POST", "/imports", gin.H{"fileName": "a.csv", "format": "template", "content": content}))
	limited := p.user("tpl-b@example.com")
	limited.Permissions[auth.ImportsRun] = true
	body := p.as(limited).must(403, "POST", "/imports/"+batch+"/approve", nil)
	if !strings.Contains(fmt.Sprint(body), "IMPORT_NEEDS_ALLOCATE") {
		t.Fatalf("got %v", body)
	}
}
