package admin

import (
	"context"
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

var (
	errProtected = httpx.NewError(http.StatusForbidden, "ROLE_PROTECTED", "The Super Administrator role always holds every permission.")
	errOwnRole   = httpx.NewError(http.StatusForbidden, "SELF_ROLE_CHANGE_DENIED", "Another administrator must change a role you hold.")
	errLockout   = httpx.NewError(http.StatusConflict, "LAST_ADMINISTRATOR", "This would leave nobody able to manage users.")
	errRoleInUse = httpx.NewError(http.StatusConflict, "ROLE_IN_USE", "Remove this role from its users first.")
	errSystem    = httpx.NewError(http.StatusConflict, "ROLE_IS_SYSTEM", "Default roles can be edited but not deleted.")
	errNameTaken = httpx.NewError(http.StatusConflict, "NAME_TAKEN", "Another role has this name.")
)

// MatrixRole is a role with its permissions and how many people hold it.
type MatrixRole struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	IsSystem    bool     `json:"isSystem"`
	Locked      bool     `json:"locked"`
	Customised  bool     `json:"isCustomised"`
	UserCount   int64    `json:"userCount"`
	Permissions []string `json:"permissions"`
}

// Matrix is the whole permission matrix.
type Matrix struct {
	Permissions []string     `json:"permissions"`
	Roles       []MatrixRole `json:"roles"`
}

func (h *Handler) matrix(c *gin.Context) {
	ctx := c.Request.Context()
	rows, err := h.conn.Query(ctx, `SELECT resource || ':' || action FROM permissions ORDER BY resource, action`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	permissions, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err = h.conn.Query(ctx, `
		SELECT r.id::text, r.name, r.description, r.is_system, r.is_customised, (SELECT count(DISTINCT ur.user_id) FROM user_roles ur WHERE ur.role_id = r.id),
		  COALESCE((SELECT array_agg(p.resource || ':' || p.action ORDER BY p.resource, p.action) FROM role_permissions rp
		            JOIN permissions p ON p.id = rp.permission_id WHERE rp.role_id = r.id), '{}')
		FROM roles r ORDER BY r.is_system DESC, r.name`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	roles, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (MatrixRole, error) {
		var m MatrixRole
		err := r.Scan(&m.ID, &m.Name, &m.Description, &m.IsSystem, &m.Customised, &m.UserCount, &m.Permissions)
		m.Locked = m.Name == auth.SuperAdminRole
		return m, err
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, Matrix{permissions, roles})
}

func (h *Handler) createRole(c *gin.Context) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
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
	var id string
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO roles (name, description, is_customised) VALUES ($1, $2, true) RETURNING id::text`, req.Name, req.Description).Scan(&id)
		if db.ErrorCode(err) == "23505" {
			return errNameTaken
		}
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "role.create", ObjectType: "role", ObjectID: id, New: req})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusCreated, gin.H{"id": id, "name": req.Name, "description": req.Description})
}

func lockRole(ctx context.Context, tx pgx.Tx, id string) (name string, system bool, err error) {
	err = tx.QueryRow(ctx, `SELECT name, is_system FROM roles WHERE id = $1::uuid FOR UPDATE`, id).Scan(&name, &system)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, httpx.ErrNotFound
	}
	return name, system, err
}

func (h *Handler) deleteRole(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	ctx, actor := c.Request.Context(), auth.Current(c).UserID
	err := db.InTx(ctx, h.conn, func(tx pgx.Tx) error {
		name, system, err := lockRole(ctx, tx, id)
		if err != nil {
			return err
		}
		if system {
			return errSystem
		}
		var used bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE role_id = $1::uuid)
			OR EXISTS (SELECT 1 FROM approval_workflow_steps WHERE approver_role_id = $1::uuid)
			OR EXISTS (SELECT 1 FROM ip_request_approvals WHERE approver_role_id = $1::uuid)`, id).Scan(&used); err != nil {
			return err
		}
		if used {
			return errRoleInUse
		}
		if _, err := tx.Exec(ctx, `DELETE FROM role_permissions WHERE role_id = $1::uuid`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM roles WHERE id = $1::uuid`, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "role.delete", ObjectType: "role", ObjectID: id, Old: gin.H{"name": name}})
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
