package admin

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// updateRole changes a role's name and description. Default roles keep their names,
// because the seed and setup steps find them by name.
func (h *Handler) updateRole(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req roleUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 100 {
		httpx.Fail(c, httpx.Validation("name", "Enter a name of up to 100 characters."))
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error { return saveRole(ctx, tx, actor, id, req) })
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type roleUpdate struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func saveRole(ctx context.Context, tx pgx.Tx, actor, id string, req roleUpdate) error {
	name, system, err := lockRole(ctx, tx, id)
	if err != nil {
		return err
	}
	if system && req.Name != name {
		return httpx.Validation("name", "Default roles keep their names. Their descriptions can change.")
	}
	var old string
	if err := tx.QueryRow(ctx, `SELECT description FROM roles WHERE id = $1::uuid`, id).Scan(&old); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE roles SET name = $2, description = $3, is_customised = true WHERE id = $1::uuid`, id, req.Name, req.Description)
	if db.ErrorCode(err) == "23505" {
		return errNameTaken
	}
	if err != nil {
		return err
	}
	return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "role.update", ObjectType: "role", ObjectID: id,
		Old: gin.H{"name": name, "description": old}, New: req})
}

// setPermissions replaces a role's permissions. Separation of duties and the lockout
// check both run inside the transaction, against the state after the change.
func (h *Handler) setPermissions(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req struct {
		Permissions []string `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Permissions == nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	slices.Sort(req.Permissions)
	req.Permissions = slices.Compact(req.Permissions)
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error { return replacePermissions(ctx, tx, actor, id, req.Permissions) })
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func replacePermissions(ctx context.Context, tx pgx.Tx, actor, id string, permissions []string) error {
	// One role change at a time, so two edits can't each pass the lockout check.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('role-permissions', 7))`); err != nil {
		return err
	}
	name, err := checkRoleEditable(ctx, tx, actor, id)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT p.resource || ':' || p.action FROM role_permissions rp JOIN permissions p ON p.id = rp.permission_id
		WHERE rp.role_id = $1::uuid ORDER BY 1`, id)
	if err != nil {
		return err
	}
	old, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	if err := writePermissions(ctx, tx, id, permissions); err != nil {
		return err
	}
	if err := checkNoLockout(ctx, tx); err != nil {
		return err
	}
	return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "role.set_permissions", ObjectType: "role", ObjectID: id,
		Old: gin.H{"permissions": old}, New: gin.H{"role": name, "permissions": permissions}})
}

// checkRoleEditable refuses the protected role and any role the actor holds.
func checkRoleEditable(ctx context.Context, tx pgx.Tx, actor, id string) (string, error) {
	name, _, err := lockRole(ctx, tx, id)
	if err != nil {
		return "", err
	}
	if name == auth.SuperAdminRole {
		return "", errProtected
	}
	var holds bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1::uuid AND role_id = $2::uuid)`, actor, id).Scan(&holds); err != nil {
		return "", err
	}
	if holds {
		return "", errOwnRole
	}
	return name, nil
}

func writePermissions(ctx context.Context, tx pgx.Tx, id string, permissions []string) error {
	var known int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM permissions WHERE resource || ':' || action = ANY($1)`, permissions).Scan(&known); err != nil {
		return err
	}
	if known != len(permissions) {
		return httpx.Validation("permissions", "Use only known permissions.")
	}
	if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1::uuid`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO role_permissions (role_id, permission_id)
		SELECT $1::uuid, id FROM permissions WHERE resource || ':' || action = ANY($2)`, id, permissions); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE roles SET is_customised = true WHERE id = $1::uuid`, id)
	return err
}

func checkNoLockout(ctx context.Context, tx pgx.Tx) error {
	var admins int
	if err := tx.QueryRow(ctx, `SELECT count(DISTINCT u.id) FROM users u JOIN user_roles ur ON ur.user_id = u.id
		JOIN role_permissions rp ON rp.role_id = ur.role_id JOIN permissions p ON p.id = rp.permission_id
		WHERE u.is_active AND p.resource = 'users' AND p.action = 'manage'`).Scan(&admins); err != nil {
		return err
	}
	if admins == 0 {
		return errLockout
	}
	return nil
}
