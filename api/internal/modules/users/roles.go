package users

import (
	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Role struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *Handler) roles(c *gin.Context) {
	rows, err := h.pool.Query(c.Request.Context(), `SELECT id::text, name, description FROM roles ORDER BY name`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Role, error) {
		var role Role
		return role, r.Scan(&role.ID, &role.Name, &role.Description)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, httpx.Page{Number: 1, Size: len(out)}, int64(len(out)))
}

// setRoles replaces a user's roles. Business services stay with a role that is kept,
// and go with one that is taken away. Nobody edits their own roles, which
// prevents both self-promotion and accidental lockout.
func (h *Handler) setRoles(c *gin.Context) {
	target, actor := c.Param("id"), auth.Current(c).UserID
	if target == actor {
		httpx.Fail(c, httpx.NewError(http.StatusForbidden, "SELF_ROLE_CHANGE_DENIED", "Another administrator must change your roles."))
		return
	}
	var req struct {
		RoleIDs []string `json:"roleIds"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	ctx := c.Request.Context()
	err := db.InTx(ctx, h.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1::uuid
			AND (business_service_id IS NULL OR role_id <> ALL($2::text[]::uuid[]))`, target, req.RoleIDs); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) SELECT $1::uuid, role::uuid FROM unnest($2::text[]) AS t(role)`, target, req.RoleIDs); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "user.set_roles", ObjectType: "user", ObjectID: target, New: req})
	})
	if code := db.ErrorCode(err); code == db.CodeForeignKey || code == "22P02" {
		httpx.Fail(c, httpx.Validation("roleIds", "A user or role was not found."))
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
