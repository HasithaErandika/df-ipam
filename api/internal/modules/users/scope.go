package users

import (
	"net/http"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// scopedRole joins a user_roles row, aliased x, to the permissions limited to business services.
const scopedRole = `JOIN role_permissions srp ON srp.role_id = x.role_id JOIN permissions spm ON spm.id = srp.permission_id
	AND ((spm.resource = 'inventory' AND spm.action = 'view_scoped') OR (spm.resource = 'reports' AND spm.action = 'export_scoped'))`

// setScope replaces the business services a user's scoped roles cover. Like roles,
// nobody changes their own, because scope decides what they can read.
func (h *Handler) setScope(c *gin.Context) {
	target, actor := c.Param("id"), auth.Current(c).UserID
	if target == actor {
		httpx.Fail(c, httpx.NewError(http.StatusForbidden, "SELF_ROLE_CHANGE_DENIED", "Another administrator must change your access."))
		return
	}
	var req struct {
		BusinessServiceIDs []string `json:"businessServiceIds"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	ctx := c.Request.Context()
	err := db.InTx(ctx, h.pool, func(tx pgx.Tx) error {
		var scoped bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles x `+scopedRole+` WHERE x.user_id = $1::uuid)`, target).Scan(&scoped); err != nil {
			return err
		}
		if !scoped {
			return httpx.Validation("businessServiceIds", "This person has no role limited to business services.")
		}
		if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1::uuid AND business_service_id IS NOT NULL`, target); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO user_roles (user_id, role_id, business_service_id)
			SELECT $1::uuid, r.role_id, bs::uuid FROM (SELECT DISTINCT x.role_id FROM user_roles x `+scopedRole+` WHERE x.user_id = $1::uuid) r
			CROSS JOIN unnest($2::text[]) AS bs`, target, req.BusinessServiceIDs); err != nil {
			return err
		}
		return audit.Write(ctx, tx, audit.Entry{ActorID: actor, Action: "user.set_scope", ObjectType: "user", ObjectID: target, New: req})
	})
	if code := db.ErrorCode(err); code == db.CodeForeignKey || code == "22P02" {
		httpx.Fail(c, httpx.Validation("businessServiceIds", "A business service was not found."))
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
