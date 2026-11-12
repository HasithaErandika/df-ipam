package auth

import "testing"

func TestPrincipalTakesOnlyKnownPermissions(t *testing.T) {
	p := NewPrincipal("u", "u@example.com", "viewer", []string{InventoryView, "made:up"}, false)
	if !p.Has(InventoryView) || p.Has("made:up") || p.Has(InventoryEdit) {
		t.Fatalf("got %v", p.Permissions)
	}
}

func TestFullAccessHoldsEveryPermission(t *testing.T) {
	p := NewPrincipal("u", "u@example.com", "admin", nil, true)
	for _, name := range AllPermissions {
		if !p.Has(name) {
			t.Errorf("full access lacks %s", name)
		}
	}
}

func TestNoRoleHasNothing(t *testing.T) {
	if p := NewPrincipal("u", "u@example.com", "", nil, false); len(p.Permissions) != 0 {
		t.Fatalf("got %v", p.Permissions)
	}
}
