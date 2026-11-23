package admin

import (
	"encoding/json"
	"net/http"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// applyRoleUpdate saves the role under the user-manager lock. A system role
// keeps its permissions; any other may only be given what the editor holds.
func (h *Handler) applyRoleUpdate(c *gin.Context, tx pgx.Tx, key string, in roleInput) error {
	if err := lockUserManagers(c, tx); err != nil {
		return err
	}
	var old []byte
	var system bool
	var held []string
	if err := tx.QueryRow(c, "SELECT to_jsonb(r), r.is_system, r.permissions FROM roles r WHERE r.key = $1 FOR UPDATE", key).Scan(&old, &system, &held); err != nil {
		return err
	}
	if !system {
		if err := checkRoleGrant(c, key, held, in.Permissions); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(c, `UPDATE roles SET name = $2, description = $3,
		permissions = CASE WHEN is_system THEN permissions ELSE $4 END WHERE key = $1`, key, in.Name, in.Description, in.Permissions); err != nil {
		return err
	}
	if err := h.keepUserManager(c, tx); err != nil {
		return err
	}
	return record(c, tx, "update", "role", key, json.RawMessage(old), in)
}

// keepUserManager refuses a change that would leave nobody active who can
// manage users, so the system can't lock everyone out.
func (h *Handler) keepUserManager(c *gin.Context, tx pgx.Tx) error {
	var n int
	if err := tx.QueryRow(c, "SELECT count(*) FROM users u WHERE u.is_active AND "+auth.HolderSQL("$1"), auth.UsersManage).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return httpx.NewError(http.StatusConflict, "LAST_ADMIN", "Nobody active could manage users after this change. Keep that permission on someone's role.")
	}
	return nil
}
