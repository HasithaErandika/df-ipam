package admin

import (
	"errors"
	"net/http/httptest"
	"testing"

	"dfipam/api/internal/auth"

	"github.com/gin-gonic/gin"
)

func asManager(perms ...string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("principal", auth.NewPrincipal("u1", "m@example.com", "manager", perms, false))
	return c
}

// A user manager can neither give nor take away a permission they lack, so
// editing a role can't strip access from people who hold more.
func TestRoleGrantChecksAddedAndRemoved(t *testing.T) {
	c := asManager(auth.UsersManage, auth.InventoryView)
	old := []string{auth.SystemManage, auth.InventoryView}
	if err := checkRoleGrant(c, "it_manager", old, []string{auth.InventoryView}); !errors.Is(err, errBeyondOwn) {
		t.Fatalf("removing system:manage: want BEYOND_OWN_ACCESS, got %v", err)
	}
	if err := checkRoleGrant(c, "it_manager", old, append(old, auth.AuditView)); !errors.Is(err, errBeyondOwn) {
		t.Fatalf("adding audit:view: want BEYOND_OWN_ACCESS, got %v", err)
	}
	if err := checkRoleGrant(c, "viewer", []string{auth.InventoryView}, nil); err != nil {
		t.Fatalf("removing a held permission: %v", err)
	}
}
