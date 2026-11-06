package server

import (
	"bytes"
	"context"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/integrations"
	"dfipam/api/internal/modules/reconciliation"
	"dfipam/api/internal/platform/safehttp"
	"dfipam/api/internal/platform/secrets"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestIntegrationsNeverChangeOwnershipOrLeakSecrets(t *testing.T) {
	p, tx := setupPeople(t)
	ctx := context.Background()
	admin := p.user("p5admin@example.com", auth.SuperAdminRole)
	viewer := p.user("p5viewer@example.com", "Business Viewer")
	c := p.as(admin)

	org := id(c.must(201, "POST", "/organisations", gin.H{"name": "P5 Org", "code": "p5"}))
	rd := id(c.must(201, "POST", "/routing-domains", gin.H{"organisationId": org, "name": "P5 VRF", "kind": "vrf"}))
	c.must(201, "POST", "/subnets", gin.H{"routingDomainId": rd, "cidr": "10.55.0.0/24", "name": "P5", "environment": "production"})
	addr := id(c.must(201, "POST", "/ip-addresses", gin.H{"routingDomainId": rd, "address": "10.55.0.10", "ownerId": admin.UserID, "allocationState": "allocated"}))

	// URLs must be https, on the allowlist. Connectors still waiting can't be added.
	base := gin.H{"kind": "fortigate", "name": "edge", "credentials": gin.H{"apiToken": "super-secret-token"}}
	for _, bad := range []string{"http://fw.test.example", "https://evil.example", "https://169.254.169.254"} {
		base["baseUrl"] = bad
		c.must(422, "POST", "/integrations/connections", base)
	}
	if out := c.must(422, "POST", "/integrations/connections", gin.H{"kind": "dns", "name": "dns"}); errCode(out) != "CONNECTOR_NOT_AVAILABLE" {
		t.Fatalf("dns: %v", out)
	}
	base["baseUrl"] = "https://fw.test.example"
	base["routingDomains"] = gin.H{"vdom:root": rd}
	created := c.must(201, "POST", "/integrations/connections", base)
	conn := id(created)

	// The credential is write-only: not in the response, audit log or plain in the table.
	raw, _ := json.Marshal(created)
	if strings.Contains(string(raw), "super-secret-token") || data(created)["hasCredentials"] != true {
		t.Fatalf("response: %s", raw)
	}
	listed, _ := json.Marshal(c.must(200, "GET", "/integrations/connections", nil))
	if strings.Contains(string(listed), "super-secret-token") || strings.Contains(string(listed), "ciphertext") {
		t.Fatalf("list: %s", listed)
	}
	if n := count(t, tx, `SELECT count(*) FROM audit_logs WHERE object_id = $1 AND new_values::text LIKE '%super-secret-token%'`, conn); n != 0 {
		t.Fatal("the audit log holds the token")
	}
	if n := count(t, tx, `SELECT count(*) FROM integration_connections WHERE id = $1::uuid AND position('super-secret-token'::bytea IN credential_ciphertext) > 0`, conn); n != 0 {
		t.Fatal("the table holds the token in plain text")
	}

	// People without integration rights can't see any of it.
	p.as(viewer).must(403, "GET", "/integrations/connections", nil)
	p.as(admin)

	// One sync at a time.
	c.must(202, "POST", "/integrations/connections/"+conn+"/sync", nil)
	if out := c.must(409, "POST", "/integrations/connections/"+conn+"/sync", nil); errCode(out) != "SYNC_ACTIVE" {
		t.Fatalf("second sync: %v", out)
	}

	var seenCreds map[string]string
	result := &integrations.Result{
		Devices: []integrations.Device{{ExternalID: "fw1", Name: "fw1", DeviceType: "firewall", VDOMs: []integrations.VDOM{{Name: "root", Context: "vdom:root"}},
			Interfaces: []integrations.Interface{{Name: "port1", VDOM: "root", Context: "vdom:root"}}}},
		Observations: []integrations.Observation{
			{Source: integrations.SourceFortiGate, Context: "vdom:root", Address: "10.55.0.10", MAC: "00:11:22:33:44:55", Alive: true},
			{Source: integrations.SourceFortiGate, Context: "vdom:root", Address: "10.55.0.77", Alive: true},
			{Source: integrations.SourceVMware, Context: "vdom:root", Address: "10.55.0.78", Payload: map[string]any{"stale": true}},
			{Source: integrations.SourceFortiGate, Context: "vdom:unmapped", Address: "10.55.0.10", Alive: true},
		},
		Warnings: []integrations.Warning{{Code: "ARP_UNAVAILABLE", Message: "denied"}},
	}
	svc := integrations.NewService(tx, testKeys(t), testPolicy).WithBuilder(func(_ integrations.Connection, creds map[string]string, _ safehttp.Policy) (integrations.Connector, error) {
		seenCreds = creds
		return fixedConnector{result: result}, nil
	})
	res, err := svc.RunSyncs(ctx)
	if err != nil || res.Completed != 1 || res.Failed != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	if seenCreds["apiToken"] != "super-secret-token" {
		t.Fatal("the worker couldn't decrypt the credential")
	}

	// Evidence moved the observation fields, and nothing else.
	row := data(c.must(200, "GET", "/ip-addresses/"+addr, nil))
	if row["allocationState"] != "allocated" || row["observationState"] != "observed_active" {
		t.Fatalf("record: %v", row)
	}
	if got := text(t, tx, `SELECT owner_id::text FROM ip_addresses WHERE id = $1::uuid`, addr); got != admin.UserID {
		t.Fatalf("owner changed to %s", got)
	}
	// An unknown address stays evidence, and creates no record.
	if n := count(t, tx, `SELECT count(*) FROM ip_addresses WHERE routing_domain_id = $1::uuid AND address = '10.55.0.77'`, rd); n != 0 {
		t.Fatal("a sync created an IP record")
	}
	// Unmapped evidence is kept, but stays outside every routing domain.
	if n := count(t, tx, `SELECT count(*) FROM discovery_observations o JOIN integration_sync_jobs j ON j.id = o.sync_job_id WHERE j.connection_id = $1::uuid AND o.routing_domain_id IS NULL`, conn); n != 1 {
		t.Fatalf("unmapped evidence: %d", n)
	}
	if got := data(c.must(200, "GET", "/integrations/connections/"+conn, nil))["health"]; got != "degraded" {
		t.Fatalf("a warning should degrade health: %v", got)
	}
	// A repeat sync updates the device instead of duplicating it.
	c.must(202, "POST", "/integrations/connections/"+conn+"/sync", nil)
	if _, err := svc.RunSyncs(ctx); err != nil {
		t.Fatal(err)
	}
	if n := count(t, tx, `SELECT count(*) FROM devices WHERE source_system = 'fortigate' AND external_id = $1`, conn+":fw1"); n != 1 {
		t.Fatalf("devices: %d", n)
	}

	// Stale VMware data never raises a finding as if it were active.
	if _, err := reconciliation.Run(ctx, tx, admin.UserID); err != nil {
		t.Fatal(err)
	}
	if n := count(t, tx, `SELECT count(*) FROM reconciliation_results WHERE classification = 'unknown_ip' AND routing_domain_id = $1::uuid AND host(address) = '10.55.0.77'`, rd); n != 1 {
		t.Fatalf("unknown address finding: %d", n)
	}
	if n := count(t, tx, `SELECT count(*) FROM reconciliation_results WHERE routing_domain_id = $1::uuid AND host(address) = '10.55.0.78'`, rd); n != 0 {
		t.Fatal("stale evidence raised a finding")
	}
	// The score is calculated, capped and explained.
	if got := text(t, tx, `SELECT confidence_breakdown->>'cap' FROM reconciliation_results WHERE classification = 'unknown_ip' AND host(address) = '10.55.0.77'`); got != "90" {
		t.Fatalf("breakdown cap: %s", got)
	}

	// A failed sync is recorded, and tells the people who manage integrations.
	failing := integrations.NewService(tx, testKeys(t), testPolicy).WithBuilder(func(integrations.Connection, map[string]string, safehttp.Policy) (integrations.Connector, error) {
		return fixedConnector{err: errors.New("connection refused")}, nil
	})
	c.must(202, "POST", "/integrations/connections/"+conn+"/sync", nil)
	if res, err := failing.RunSyncs(ctx); err != nil || res.Failed != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	got := data(c.must(200, "GET", "/integrations/connections/"+conn, nil))
	if got["health"] != "failed" || got["lastError"] != "connection refused" {
		t.Fatalf("after failure: %v", got)
	}
	if n := count(t, tx, `SELECT count(*) FROM notifications WHERE event_type = 'integration_failed' AND recipient_id = $1::uuid`, admin.UserID); n == 0 {
		t.Fatal("nobody was told")
	}

	// History is evidence, so a connection that has synced can't be deleted.
	if out := c.must(409, "DELETE", "/integrations/connections/"+conn, nil); errCode(out) != "CONNECTION_HAS_HISTORY" {
		t.Fatalf("delete: %v", out)
	}

	// Rotation moves every credential to the new key, and it still decrypts.
	rotated, err := secrets.NewKeyring(2, map[int][]byte{1: bytes.Repeat([]byte{7}, 32), 2: bytes.Repeat([]byte{9}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := integrations.NewService(tx, rotated, testPolicy).RotateCredentials(ctx); err != nil || n < 1 {
		t.Fatalf("%d %v", n, err)
	}
	var sealed []byte
	if err := tx.QueryRow(ctx, `SELECT credential_ciphertext FROM integration_connections WHERE id = $1::uuid`, conn).Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	newOnly, _ := secrets.NewKeyring(2, map[int][]byte{2: bytes.Repeat([]byte{9}, 32)})
	plain, err := newOnly.Decrypt(sealed, 2, []byte(conn))
	if err != nil || !strings.Contains(string(plain), "super-secret-token") {
		t.Fatalf("after rotation: %v", err)
	}
}
