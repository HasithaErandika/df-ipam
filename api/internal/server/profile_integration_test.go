package server

import (
	"testing"

	"dfipam/api/internal/auth"
)

// People see their own account, and never the provider's subject identifier.
func TestProfileShowsOwnAccount(t *testing.T) {
	p, _ := setupPeople(t)
	c := p.as(p.user("profile@example.com", auth.SuperAdminRole))
	got := data(c.must(200, "GET", "/auth/me/profile", nil))
	if got["email"] != "profile@example.com" || got["isActive"] != true || got["createdAt"] == nil {
		t.Fatalf("profile: %v", got)
	}
	if _, leaked := got["authSubject"]; leaked {
		t.Fatalf("profile exposes the subject: %v", got)
	}
	roles := got["assignments"].([]any)
	if len(roles) != 1 || roles[0].(map[string]any)["role"] != auth.SuperAdminRole || roles[0].(map[string]any)["scope"] != "" {
		t.Fatalf("assignments: %v", roles)
	}
}
