package auth

import "slices"

// Permissions are checked by name; roles, set up by Admins in the roles table,
// only group them. Add a permission here and in web/src/lib/permissions.ts.
const (
	InventoryView   = "inventory:view"
	InventoryEdit   = "inventory:edit"
	NetworkManage   = "network:manage"
	DiscoveryScan   = "discovery:scan"
	IPRequest       = "ip:request"
	IPReserve       = "ip:reserve"
	RequestsApprove = "requests:approve"
	AlertsManage    = "alerts:manage"
	ReportsExport   = "reports:export"
	AuditView       = "audit:view"
	UsersManage     = "users:manage"
	SystemManage    = "system:manage"
)

// AllPermissions lists every permission, in a stable order.
var AllPermissions = []string{
	InventoryView, InventoryEdit, NetworkManage, DiscoveryScan, IPRequest, IPReserve, RequestsApprove,
	AlertsManage, ReportsExport, AuditView, UsersManage, SystemManage,
}

// KnownPermission reports whether name is a permission the API checks.
func KnownPermission(name string) bool { return slices.Contains(AllPermissions, name) }

// HolderSQL is a condition on users u that holds when the user's role grants
// the permission in parameter $n. Use it to reach "everyone who may do X"
// instead of naming a role.
func HolderSQL(n string) string {
	return `EXISTS (SELECT 1 FROM roles hr WHERE hr.key = u.role AND (hr.full_access OR ` + n + ` = ANY(hr.permissions)))`
}
