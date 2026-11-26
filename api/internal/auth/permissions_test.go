package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

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

// On HTTPS only the __Host- cookie signs a request in.
func TestSessionTokenOnHTTPSIgnoresPlainCookie(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.AddCookie(&http.Cookie{Name: SessionCookiePlain, Value: "planted"})
	if got := SessionToken(c, true); got != "" {
		t.Fatalf("HTTPS read the plain cookie: %q", got)
	}
	if got := SessionToken(c, false); got != "planted" {
		t.Fatalf("plain HTTP: got %q", got)
	}
}
