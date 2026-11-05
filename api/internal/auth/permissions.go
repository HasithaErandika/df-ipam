package auth

// Permissions are checked by name. Roles only group them.
const (
	InventoryView         = "inventory:view"
	InventoryViewScoped   = "inventory:view_scoped"
	InventoryCreate       = "inventory:create"
	InventoryEdit         = "inventory:edit"
	InventoryEditAssigned = "inventory:edit_assigned"
	InventoryDelete       = "inventory:delete"
	IPRequest             = "ip:request"
	IPAllocate            = "ip:allocate"
	IPReserve             = "ip:reserve"
	RequestsApprove       = "requests:approve"
	ImportsRun            = "imports:run"
	DiscoveryRun          = "discovery:run"
	DiscoveryManage       = "discovery:manage"
	IntegrationsManage    = "integrations:manage"
	UsersManage           = "users:manage"
	AuditView             = "audit:view"
	ReportsExport         = "reports:export"
	ReportsExportScoped   = "reports:export_scoped"
)
