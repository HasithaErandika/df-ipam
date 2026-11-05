package server

import (
	"context"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/reports"
	"dfipam/api/internal/modules/subnets"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

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

	// A day no worker has reached, because the running worker snapshots today.
	day := time.Now().AddDate(0, 0, 2)
	if n, err := subnets.SnapshotDay(ctx, tx, day); err != nil || n == 0 {
		t.Fatalf("%d %v", n, err)
	}
	if n, err := subnets.SnapshotDay(ctx, tx, day); err != nil || n != 0 {
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
