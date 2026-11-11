package auth

import (
	"slices"
	"testing"
)

func TestRolesUseKnownPermissions(t *testing.T) {
	for role, perms := range RolePermissions {
		for _, p := range perms {
			if !slices.Contains(AllPermissions, p) {
				t.Errorf("%s has unknown permission %s", role, p)
			}
		}
	}
}

func TestLeastPrivilege(t *testing.T) {
	for role, denied := range map[string][]string{
		RoleViewer:    {InventoryEdit, IPRequest, IPReserve, RequestsApprove, AlertsManage, AuditView, UsersManage, SystemManage},
		RoleRequester: {InventoryEdit, RequestsApprove, AlertsManage, AuditView, UsersManage, SystemManage},
	} {
		p := NewPrincipal("u", "u@example.com", role)
		for _, name := range denied {
			if p.Has(name) {
				t.Errorf("%s must not have %s", role, name)
			}
		}
	}
}

func TestNoRoleHasNothing(t *testing.T) {
	if p := NewPrincipal("u", "u@example.com", ""); len(p.Permissions) != 0 {
		t.Fatalf("got %v", p.Permissions)
	}
	if ValidRole("Super Administrator") || !ValidRole(RoleAdmin) {
		t.Fatal("ValidRole")
	}
}
