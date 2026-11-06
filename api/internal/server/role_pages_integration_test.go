package server

import (
	"net/http"
	"slices"
	"testing"

	"dfipam/api/internal/auth"

	"github.com/gin-gonic/gin"
)

const (
	ipamAdmin = "IPAM Administrator"
	engineer  = "Network Engineer"
	infra     = "Infrastructure Engineer"
	auditor   = "Security Auditor"
	viewer    = "Business Viewer"
)

// The route each menu entry reads first, and who may read it. web/src/app/nav.ts shows
// the menu from the same rules, so a link never leads to a refusal.
var rolePages = []struct {
	path  string
	roles []string
}{
	{"/dashboard/summary", []string{auth.SuperAdminRole, ipamAdmin, engineer, infra, auditor}},
	{"/subnets", []string{auth.SuperAdminRole, ipamAdmin, engineer, infra, auditor}},
	{"/reservations", []string{auth.SuperAdminRole, ipamAdmin, engineer, infra, auditor}},
	{"/reconciliation/summary", []string{auth.SuperAdminRole, ipamAdmin, engineer, infra, auditor}},
	{"/requests", []string{auth.SuperAdminRole, ipamAdmin, engineer, infra, auditor}},
	{"/discovery/status", []string{auth.SuperAdminRole, ipamAdmin, engineer, auditor}},
	{"/imports", []string{auth.SuperAdminRole, ipamAdmin, engineer, auditor}},
	{"/integrations/connections", []string{auth.SuperAdminRole, ipamAdmin, auditor}},
	{"/audit-logs", []string{auth.SuperAdminRole, ipamAdmin, auditor}},
	{"/admin/permission-matrix", []string{auth.SuperAdminRole, ipamAdmin, auditor}},
	{"/system/retention", []string{auth.SuperAdminRole, ipamAdmin, auditor}},
	{"/users", []string{auth.SuperAdminRole}},
	{"/reports/types", []string{auth.SuperAdminRole, ipamAdmin, engineer, infra, auditor, viewer}},
	{"/notifications", []string{auth.SuperAdminRole, ipamAdmin, engineer, infra, auditor, viewer}},
}

func TestRolesReachOnlyTheirPages(t *testing.T) {
	p, _ := setupPeople(t)
	for _, role := range []string{auth.SuperAdminRole, ipamAdmin, engineer, infra, auditor, viewer} {
		c := p.as(p.user(role+"@pages.example.com", role))
		for _, page := range rolePages {
			want := http.StatusForbidden
			if slices.Contains(page.roles, role) {
				want = http.StatusOK
			}
			if code, _ := c.call("GET", page.path, nil); code != want {
				t.Errorf("%s on %s: %d, want %d", role, page.path, code, want)
			}
		}
	}
}

func TestOnlySystemManagersChangeRetention(t *testing.T) {
	p, _ := setupPeople(t)
	body := gin.H{"retainDays": 200}
	p.as(p.user("ret-admin@example.com", ipamAdmin)).must(403, "PUT", "/system/retention/notifications", body)
	p.as(p.user("ret-auditor@example.com", auditor)).must(403, "PUT", "/system/retention/notifications", body)
	root := p.as(p.user("ret-root@example.com", auth.SuperAdminRole))
	code, _ := root.call("PUT", "/system/retention/notifications", body)
	if code >= 300 {
		t.Fatalf("a system manager should change retention: %d", code)
	}
	root.must(422, "PUT", "/system/retention/audit_logs", gin.H{"retainDays": 30})
}
