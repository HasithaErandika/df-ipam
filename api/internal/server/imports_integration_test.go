package server

import (
	"context"
	"strings"
	"testing"

	"dfipam/api/internal/auth"

	"github.com/gin-gonic/gin"
)

func TestImportFromUploadToReversal(t *testing.T) {
	p, tx := setupPeople(t)
	ctx := context.Background()
	uploader := p.user("imp-up@example.com", auth.SuperAdminRole)
	approver := p.user("imp-ok@example.com", auth.SuperAdminRole)
	c := p.as(uploader)
	org := id(c.must(201, "POST", "/organisations", gin.H{"name": "Imp Org", "code": "imp"}))
	rd := id(c.must(201, "POST", "/routing-domains", gin.H{"organisationId": org, "name": "Imp VRF", "kind": "vrf"}))
	c.must(201, "POST", "/subnets", gin.H{"routingDomainId": rd, "cidr": "10.88.1.0/24", "name": "Existing", "environment": "production"})
	c.must(201, "POST", "/subnets", gin.H{"routingDomainId": rd, "cidr": "10.88.2.0/24", "name": "Wide", "environment": "production"})
	var device string
	if err := tx.QueryRow(ctx, `INSERT INTO devices (name, device_type) VALUES ('imp-fw', 'firewall') RETURNING id::text`).Scan(&device); err != nil {
		t.Fatal(err)
	}
	var vdom, vlan string
	if err := tx.QueryRow(ctx, `INSERT INTO vdoms (device_id, name, routing_domain_id) VALUES ($1::uuid, 'imp-vdom', $2::uuid) RETURNING id::text`, device, rd).Scan(&vdom); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `INSERT INTO vlans (vlan_id, name) VALUES (3888, 'Imp') RETURNING id::text`).Scan(&vlan); err != nil {
		t.Fatal(err)
	}

	content := strings.Join([]string{
		"# networks", // 1 ignored
		"10.88.10.0/24,gw=10.88.10.1,name=Alpha,vdom=imp-vdom,vlan=3888", // 2 valid
		"10.88.11.0/24,name=Beta",       // 3 valid
		"10.88.11.0/24,name=Beta again", // 4 duplicate of line 3
		"10.88.1.0/24,name=Exact",       // 5 duplicate of the inventory
		"10.88.2.128/25,name=Inside",    // 6 overlap
		"10.88.12.0/24",                 // 7 needs a name
		"10.88.13.5/24,name=Host bits",  // 8 invalid
		"nonsense",                      // 9 invalid
	}, "\n")
	batch := data(c.must(201, "POST", "/imports", gin.H{"fileName": "n.csv", "format": "csv", "content": content, "routingDomainId": rd, "environment": "production"}))
	batchID := batch["id"].(string)
	want := map[string]float64{"valid": 2, "duplicate": 2, "overlap": 1, "needsEnrichment": 1, "invalid": 2, "ignored": 1}
	summary := batch["summary"].(map[string]any)
	for k, v := range want {
		if summary[k] != v {
			t.Fatalf("%s: %v in %v", k, summary[k], summary)
		}
	}
	if n := count(t, tx, `SELECT count(*) FROM import_records WHERE batch_id = $1::uuid`, batchID); n != 9 {
		t.Fatalf("records: %d", n)
	}
	if got := text(t, tx, `SELECT errors->>0 FROM import_records WHERE batch_id = $1::uuid AND line_number = 4`, batchID); got != "Same network as line 3." {
		t.Fatalf("line 4: %s", got)
	}
	if got := text(t, tx, `SELECT errors->>0 FROM import_records WHERE batch_id = $1::uuid AND line_number = 5`, batchID); !strings.Contains(got, "Already in the inventory: Existing") {
		t.Fatalf("line 5: %s", got)
	}
	if got := text(t, tx, `SELECT errors->>0 FROM import_records WHERE batch_id = $1::uuid AND line_number = 6`, batchID); !strings.Contains(got, "Overlaps Wide") {
		t.Fatalf("line 6: %s", got)
	}
	if got := text(t, tx, `SELECT raw_text FROM import_records WHERE batch_id = $1::uuid AND line_number = 3`, batchID); got != "10.88.11.0/24,name=Beta" {
		t.Fatalf("raw text: %s", got)
	}

	// Correcting a line checks it against the rest of the file.
	line7 := text(t, tx, `SELECT id::text FROM import_records WHERE batch_id = $1::uuid AND line_number = 7`, batchID)
	if got := data(c.must(200, "PATCH", "/imports/records/"+line7, gin.H{"cidr": "10.88.10.0/24", "name": "Clash"})); got["status"] != "duplicate" {
		t.Fatalf("clash: %v", got)
	}
	if got := data(c.must(200, "PATCH", "/imports/records/"+line7, gin.H{"cidr": "10.88.12.0/24", "name": "Gamma"})); got["status"] != "valid" {
		t.Fatalf("fixed: %v", got)
	}

	if code, out := c.call("POST", "/imports/"+batchID+"/approve", nil); code != 403 {
		t.Fatalf("the uploader must not approve: %d %v", code, out)
	}
	committed := data(p.as(approver).must(200, "POST", "/imports/"+batchID+"/approve", nil))
	if committed["status"] != "committed" || committed["summary"].(map[string]any)["committed"] != float64(3) {
		t.Fatalf("committed: %v", committed)
	}
	if n := count(t, tx, `SELECT count(*) FROM subnets s JOIN import_records r ON r.created_object_id = s.id
		WHERE r.batch_id = $1::uuid AND r.status = 'committed' AND r.created_object_type = 'subnet'`, batchID); n != 3 {
		t.Fatalf("subnets: %d", n)
	}
	if n := count(t, tx, `SELECT count(*) FROM subnets WHERE routing_domain_id = $1::uuid AND name = 'Alpha' AND vdom_id = $2::uuid AND vlan_id = $3::uuid AND gateway = '10.88.10.1'`, rd, vdom, vlan); n != 1 {
		t.Fatal("the VDOM, VLAN and gateway of line 2 must be linked")
	}

	c = p.as(approver)
	reversed := data(c.must(200, "POST", "/imports/"+batchID+"/reverse", nil))
	if reversed["status"] != "reversed" {
		t.Fatalf("reversed: %v", reversed)
	}
	if n := count(t, tx, `SELECT count(*) FROM subnets WHERE routing_domain_id = $1::uuid AND name IN ('Alpha', 'Beta', 'Gamma') AND archived_at IS NULL`, rd); n != 0 {
		t.Fatalf("subnets left: %d", n)
	}
	if n := count(t, tx, `SELECT count(*) FROM import_records WHERE batch_id = $1::uuid AND status = 'valid' AND created_object_id IS NULL`, batchID); n != 3 {
		t.Fatalf("records back to valid: %d", n)
	}
}
