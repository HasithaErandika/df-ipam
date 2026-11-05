package seed

import (
	"slices"
	"testing"

	"dfipam/api/internal/auth"
)

func role(t *testing.T, name string) Role {
	t.Helper()
	for _, r := range Roles {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no role %q", name)
	return Role{}
}

func TestRolePermissionsAreKnown(t *testing.T) {
	if len(Roles) != 6 {
		t.Fatalf("want 6 roles, got %d", len(Roles))
	}
	for _, r := range Roles {
		for _, p := range r.Permissions {
			if !slices.Contains(AllPermissions, p) {
				t.Errorf("%s has unknown permission %s", r.Name, p)
			}
		}
	}
}

func TestLeastPrivilege(t *testing.T) {
	for name, denied := range map[string][]string{
		"Security Auditor":        {auth.InventoryCreate, auth.InventoryEdit, auth.InventoryDelete, auth.IPAllocate},
		"Business Viewer":         {auth.InventoryView, auth.AuditView, auth.ReportsExport},
		"Infrastructure Engineer": {auth.InventoryEdit, auth.IPAllocate, auth.RequestsApprove},
		"Network Engineer":        {auth.RequestsApprove, auth.UsersManage, auth.InventoryDelete},
		"IPAM Administrator":      {auth.UsersManage},
	} {
		for _, p := range denied {
			if slices.Contains(role(t, name).Permissions, p) {
				t.Errorf("%s must not have %s", name, p)
			}
		}
	}
}
