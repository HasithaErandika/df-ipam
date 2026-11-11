package auth

// Permissions are checked by name. The three fixed roles only group them.
const (
	InventoryView   = "inventory:view"
	InventoryEdit   = "inventory:edit"
	IPRequest       = "ip:request"
	IPReserve       = "ip:reserve"
	RequestsApprove = "requests:approve"
	AlertsManage    = "alerts:manage"
	ReportsExport   = "reports:export"
	AuditView       = "audit:view"
	UsersManage     = "users:manage"
	SystemManage    = "system:manage"
)

// Roles a user can hold. A user with no role is signed in but sees nothing.
const (
	RoleAdmin     = "admin"
	RoleRequester = "requester"
	RoleViewer    = "viewer"
)

// AllPermissions lists every permission, in a stable order.
var AllPermissions = []string{
	InventoryView, InventoryEdit, IPRequest, IPReserve, RequestsApprove,
	AlertsManage, ReportsExport, AuditView, UsersManage, SystemManage,
}

// RolePermissions follows docs/new.md: Admin changes things, Requester reads and asks
// for IPs and reservations, Viewer only reads.
var RolePermissions = map[string][]string{
	RoleAdmin:     AllPermissions,
	RoleRequester: {InventoryView, IPRequest, IPReserve, ReportsExport},
	RoleViewer:    {InventoryView, ReportsExport},
}

// ValidRole reports whether name is one of the fixed roles.
func ValidRole(name string) bool {
	_, ok := RolePermissions[name]
	return ok
}
