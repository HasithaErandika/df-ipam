// Package seed loads reference data. Every statement is safe to run repeatedly.
package seed

import (
	"context"
	"fmt"
	"strings"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/notifications"

	"github.com/jackc/pgx/v5"
)

// Role pairs a role with its default permissions, following docs/authentication.md.
type Role struct {
	Name, Description string
	Permissions       []string
}

var AllPermissions = []string{
	auth.InventoryView, auth.InventoryViewScoped, auth.InventoryCreate, auth.InventoryEdit, auth.InventoryEditAssigned,
	auth.InventoryDelete, auth.IPRequest, auth.IPAllocate, auth.IPReserve, auth.RequestsApprove, auth.ImportsRun,
	auth.DiscoveryRun, auth.DiscoveryManage, auth.IntegrationsManage, auth.UsersManage, auth.AuditView,
	auth.ReportsExport, auth.ReportsExportScoped, auth.SystemManage,
}

var Roles = []Role{
	{auth.SuperAdminRole, "Full platform configuration and user administration.", AllPermissions},
	{"IPAM Administrator", "Manages subnets, IP records, discovery policies and integrations.", []string{
		auth.InventoryView, auth.InventoryCreate, auth.InventoryEdit, auth.InventoryDelete, auth.IPRequest,
		auth.IPAllocate, auth.IPReserve, auth.RequestsApprove, auth.ImportsRun, auth.DiscoveryRun,
		auth.DiscoveryManage, auth.IntegrationsManage, auth.AuditView, auth.ReportsExport}},
	{"Network Engineer", "Manages network records and performs authorised discovery.", []string{
		auth.InventoryView, auth.InventoryCreate, auth.InventoryEdit, auth.IPRequest, auth.IPAllocate, auth.IPReserve,
		auth.ImportsRun, auth.DiscoveryRun, auth.ReportsExport}},
	{"Infrastructure Engineer", "Views inventory, manages assigned records and requests allocations.", []string{
		auth.InventoryView, auth.InventoryEditAssigned, auth.IPRequest, auth.ReportsExport}},
	{"Security Auditor", "Read-only access to inventory, audit evidence and reports.", []string{
		auth.InventoryView, auth.AuditView, auth.ReportsExport}},
	{"Business Viewer", "Read-only access to specifically permitted business-service information.", []string{
		auth.InventoryViewScoped, auth.ReportsExportScoped}},
}

var sources = []struct {
	Name, Kind  string
	Reliability int
}{
	{"Authoritative allocation", "authoritative", 100}, {"Manual inventory", "manual", 90},
	{"FortiGate", "integration", 85}, {"VMware", "integration", 70}, {"AWS", "integration", 90},
	{"Azure", "integration", 90}, {"DHCP", "integration", 70}, {"DNS", "passive", 50},
	{"SNMP", "active", 75}, {"ARP", "active", 75}, {"ICMP", "active", 40}, {"Nmap", "active", 40},
}

// Starting values for an administrator to review; not agreed organisational policy.
var retention = []struct {
	Class string
	Days  int
}{
	{"discovery_observations", 180}, {"discovery_job_logs", 90}, {"agent_heartbeats", 30},
	{"notifications", 365}, {"report_jobs", 30}, {"audit_logs", 2555},
}

// Starting weights for an administrator to review. Classifications without weights
// keep the fixed confidence in their rule, because they come from inventory alone.
// The caps are safety rules: several MACs alone never reach high confidence.
var findingPriority = `["Authoritative allocation","Manual inventory","AWS","Azure","FortiGate","SNMP","ARP","VMware","DHCP","DNS","ICMP","Nmap"]`

var findingRules = []struct{ Classification, Weights string }{
	{"new_device", `{"reliability":0.6,"freshness":0.3,"agreement":0.1,"cap":70,"freshnessDays":30}`},
	{"unknown_ip", `{"reliability":0.6,"freshness":0.3,"agreement":0.1,"cap":90,"freshnessDays":7}`},
	{"ip_conflict_suspected", `{"reliability":0.4,"freshness":0.3,"agreement":0.3,"cap":50,"freshnessDays":1}`},
	{"duplicate_mac", `{}`},
	{"dns_mismatch", `{}`},
	{"allocated_not_observed", `{}`},
	{"observed_not_allocated", `{"reliability":0.6,"freshness":0.3,"agreement":0.1,"cap":95,"freshnessDays":7}`},
	{"subnet_overlap", `{}`},
	{"ownership_missing", `{}`},
	{"infrastructure_mismatch", `{}`},
	{"stale_evidence", `{}`},
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// SQL returns the seed as plain SQL, used by Run and by the setup file.
func SQL() string {
	var b strings.Builder
	for _, p := range AllPermissions {
		resource, action, _ := strings.Cut(p, ":")
		fmt.Fprintf(&b, "INSERT INTO permissions (resource, action) VALUES (%s, %s) ON CONFLICT (resource, action) DO NOTHING;\n", quote(resource), quote(action))
	}
	for _, r := range Roles {
		fmt.Fprintf(&b, "INSERT INTO roles (name, description, is_system) VALUES (%s, %s, true) ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description WHERE NOT roles.is_customised;\n", quote(r.Name), quote(r.Description))
		for _, p := range r.Permissions {
			resource, action, _ := strings.Cut(p, ":")
			fmt.Fprintf(&b, "INSERT INTO role_permissions (role_id, permission_id) SELECT r.id, p.id FROM roles r, permissions p WHERE r.name = %s AND p.resource = %s AND p.action = %s AND NOT r.is_customised ON CONFLICT DO NOTHING;\n", quote(r.Name), quote(resource), quote(action))
		}
	}
	for _, s := range sources {
		fmt.Fprintf(&b, "INSERT INTO observation_sources (name, kind, reliability) VALUES (%s, %s, %d) ON CONFLICT (name) DO NOTHING;\n", quote(s.Name), quote(s.Kind), s.Reliability)
	}
	b.WriteString("INSERT INTO system_settings (key, value) VALUES ('discovery.emergency_stop', 'false') ON CONFLICT (key) DO NOTHING;\n")
	for _, r := range retention {
		fmt.Fprintf(&b, "INSERT INTO retention_policies (data_class, retain_days) VALUES (%s, %d) ON CONFLICT (data_class) DO NOTHING;\n", quote(r.Class), r.Days)
	}
	for _, r := range findingRules {
		fmt.Fprintf(&b, "INSERT INTO reconciliation_rules (classification, source_priority, confidence_weights) VALUES (%s, %s, %s) ON CONFLICT (classification) DO NOTHING;\n", quote(r.Classification), quote(findingPriority), quote(r.Weights))
	}
	// Every event starts switched on; administrators turn them off in the app.
	for _, e := range notifications.EventTypes {
		fmt.Fprintf(&b, "INSERT INTO notification_rules (event_type, severity, recipient_group) VALUES (%s, %s, %s) ON CONFLICT (event_type) DO NOTHING;\n", quote(e.Type), quote(e.Severity), quote(e.Recipients))
	}
	return b.String()
}

func Run(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, SQL())
	return err
}
