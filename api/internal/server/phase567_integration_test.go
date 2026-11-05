package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/integrations"
	"dfipam/api/internal/modules/reconciliation"
	"dfipam/api/internal/modules/reports"
	"dfipam/api/internal/modules/subnets"
	"dfipam/api/internal/platform/safehttp"
	"dfipam/api/internal/platform/secrets"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

var testPolicy = safehttp.Policy{Hosts: []string{"fw.test.example"}}

func testKeys(t *testing.T) *secrets.Keyring {
	t.Helper()
	k, err := secrets.NewKeyring(1, map[int][]byte{1: bytes.Repeat([]byte{7}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// fixedConnector returns canned evidence, or fails.
type fixedConnector struct {
	result *integrations.Result
	err    error
}

func (f fixedConnector) Test(context.Context) error { return f.err }
func (f fixedConnector) Collect(context.Context) (*integrations.Result, error) {
	return f.result, f.err
}

func text(t *testing.T, tx pgx.Tx, sql string, args ...any) string {
	t.Helper()
	var v string
	if err := tx.QueryRow(context.Background(), sql, args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	return v
}

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

func TestReports(t *testing.T) {
	p, tx := setupPeople(t)
	ctx := context.Background()
	admin := p.user("rep@example.com", auth.SuperAdminRole)
	viewer := p.user("repview@example.com", "Business Viewer")
	other := p.user("repother@example.com", "Security Auditor")
	c := p.as(admin)
	org := id(c.must(201, "POST", "/organisations", gin.H{"name": "Rep Org", "code": "rep"}))
	rd := id(c.must(201, "POST", "/routing-domains", gin.H{"organisationId": org, "name": "Rep VRF", "kind": "vrf"}))
	subnet := id(c.must(201, "POST", "/subnets", gin.H{"routingDomainId": rd, "cidr": "2001:db8:77::/64", "name": "=cmd|'/c calc'!A1", "environment": "production"}))
	c.must(201, "POST", "/ip-addresses", gin.H{"routingDomainId": rd, "address": "2001:db8:77::5", "allocationState": "allocated"})

	if n, err := subnets.SnapshotDay(ctx, tx, time.Now()); err != nil || n == 0 {
		t.Fatalf("%d %v", n, err)
	}
	if n, err := subnets.SnapshotDay(ctx, tx, time.Now()); err != nil || n != 0 {
		t.Fatalf("second snapshot: %d %v", n, err)
	}
	if n := count(t, tx, `SELECT count(*) FROM subnet_utilisation_snapshots WHERE subnet_id = $1::uuid AND allocated = 1`, subnet); n != 1 {
		t.Fatal("snapshot missing")
	}

	// Every report runs in every format against the real schema.
	types := c.must(200, "GET", "/reports/types", nil)["data"].([]any)
	if len(types) != len(reports.Definitions) {
		t.Fatalf("%d of %d types", len(types), len(reports.Definitions))
	}
	var register string
	for _, raw := range types {
		key := raw.(map[string]any)["key"].(string)
		for _, format := range []string{"csv", "xlsx", "pdf"} {
			job := id(c.must(202, "POST", "/reports", gin.H{"reportType": key, "format": format}))
			if res, err := reports.Run(ctx, tx); err != nil || res.Failed != 0 {
				t.Fatalf("%s %s: %+v %v", key, format, res, err)
			}
			if key == "subnet_register" && format == "csv" {
				register = job
			}
		}
	}

	req := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/api/v1"+path, nil)
		r.Header.Set("Authorization", "Bearer token")
		w := httptest.NewRecorder()
		c.engine.ServeHTTP(w, r)
		return w
	}
	p.as(admin)
	w := req("/reports/" + register + "/download")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "2001:db8:77::/64") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	// A planted formula arrives as text.
	if !strings.Contains(w.Body.String(), `'=cmd|`) {
		t.Fatalf("formula not neutralised: %s", w.Body.String())
	}
	if n := count(t, tx, `SELECT count(*) FROM audit_logs WHERE action = 'report.download' AND object_id = $1`, register); n != 1 {
		t.Fatal("download not audited")
	}
	// A report belongs to whoever requested it.
	p.as(other)
	if w := req("/reports/" + register + "/download"); w.Code != 404 {
		t.Fatalf("someone else's report: %d", w.Code)
	}

	// A scoped person with no business services gets an empty report, not everything.
	v := p.as(viewer)
	v.must(403, "POST", "/reports", gin.H{"reportType": "audit_trail", "format": "csv"})
	v.must(403, "POST", "/reports", gin.H{"reportType": "discovery_coverage", "format": "csv"})
	job := id(v.must(202, "POST", "/reports", gin.H{"reportType": "subnet_register", "format": "csv", "filters": gin.H{"scope": []string{}}}))
	if res, err := reports.Run(ctx, tx); err != nil || res.Failed != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	if n := count(t, tx, `SELECT row_count::int FROM report_jobs WHERE id = $1::uuid`, job); n != 0 {
		t.Fatalf("scoped report had %d rows", n)
	}

	// Five running reports is the limit.
	c = p.as(admin)
	for range 5 {
		c.must(202, "POST", "/reports", gin.H{"reportType": "subnet_register", "format": "csv"})
	}
	if out := c.must(429, "POST", "/reports", gin.H{"reportType": "subnet_register", "format": "csv"}); errCode(out) != "REPORT_LIMIT" {
		t.Fatalf("limit: %v", out)
	}
	c.must(422, "POST", "/reports", gin.H{"reportType": "nonsense", "format": "csv"})
	c.must(422, "POST", "/reports", gin.H{"reportType": "subnet_register", "format": "docx"})

	// Dashboard data keeps big counts as strings.
	points := c.must(200, "GET", "/dashboard/trends?days=7", nil)["data"].([]any)
	if len(points) != 7 {
		t.Fatalf("%d points", len(points))
	}
	if _, ok := points[6].(map[string]any)["allocated"].(string); !ok {
		t.Fatalf("count isn't a string: %v", points[6])
	}
	c.must(422, "GET", "/dashboard/trends?days=9999", nil)
	c.must(200, "GET", "/dashboard/distribution", nil)
	c.must(200, "GET", "/dashboard/summary", nil)

	// Audit filters.
	if got := c.must(200, "GET", "/audit-logs?action=report.download&actor=rep%40&from=2020-01-01", nil)["meta"].(map[string]any)["total"]; got != float64(1) {
		t.Fatalf("filter: %v", got)
	}
	c.must(422, "GET", "/audit-logs?from=yesterday", nil)
}

func TestAdministration(t *testing.T) {
	p, _ := setupPeople(t)
	admin := p.user("adm@example.com", auth.SuperAdminRole)
	engineer := p.user("eng@example.com", "Network Engineer")
	auditor := p.user("aud@example.com", "Security Auditor")
	c := p.as(admin)

	matrix := data(c.must(200, "GET", "/admin/permission-matrix", nil))
	roles := map[string]string{}
	for _, raw := range matrix["roles"].([]any) {
		r := raw.(map[string]any)
		roles[r["name"].(string)] = r["id"].(string)
	}
	body := gin.H{"permissions": []string{"inventory:view"}}
	if out := c.must(403, "PUT", "/roles/"+roles[auth.SuperAdminRole]+"/permissions", body); errCode(out) != "ROLE_PROTECTED" {
		t.Fatalf("super admin: %v", out)
	}
	c.must(422, "PUT", "/roles/"+roles["Network Engineer"]+"/permissions", gin.H{"permissions": []string{"made:up"}})
	c.must(204, "PUT", "/roles/"+roles["Network Engineer"]+"/permissions", body)
	if out := c.must(409, "DELETE", "/roles/"+roles["Network Engineer"], nil); errCode(out) != "ROLE_IS_SYSTEM" {
		t.Fatalf("delete system role: %v", out)
	}

	role := id(c.must(201, "POST", "/roles", gin.H{"name": "Temp role", "description": "x"}))
	c.must(409, "POST", "/roles", gin.H{"name": "Temp role"})
	c.must(204, "DELETE", "/roles/"+role, nil)

	p.as(engineer).must(403, "PUT", "/roles/"+roles["Network Engineer"]+"/permissions", body)
	p.as(engineer).must(403, "GET", "/system/health", nil)

	// An auditor can look but not change.
	a := p.as(auditor)
	health := data(a.must(200, "GET", "/system/health", nil))
	if health["database"] != "ok" || health["encryptionConfigured"] != true {
		t.Fatalf("health: %v", health)
	}
	a.must(200, "GET", "/system/retention", nil)
	a.must(403, "PUT", "/system/retention/notifications", gin.H{"retainDays": 100})

	c = p.as(admin)
	c.must(204, "PUT", "/system/retention/notifications", gin.H{"retainDays": 100})
	c.must(422, "PUT", "/system/retention/notifications", gin.H{"retainDays": 1})
	c.must(422, "PUT", "/system/retention/audit_logs", gin.H{"retainDays": 30})
	c.must(404, "PUT", "/system/retention/nonsense", gin.H{"retainDays": 100})
}
