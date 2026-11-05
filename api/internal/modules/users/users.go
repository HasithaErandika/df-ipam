// Package users exposes the signed-in user and role administration.
package users

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"sort"
	"strings"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type User struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"displayName"`
	IsActive    bool       `json:"isActive"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
	Roles       []string   `json:"roles"`
}

type Role struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Inviter sends a sign-in invitation through the auth provider.
type Inviter interface {
	Invite(ctx context.Context, email string) error
}

type Handler struct {
	pool    db.Conn
	inviter Inviter
}

func NewHandler(pool db.Conn, inviter Inviter) *Handler { return &Handler{pool, inviter} }

func (h *Handler) Register(g *auth.Guard) {
	g.Authenticated(http.MethodGet, "/auth/me", h.me)
	g.Handle(http.MethodGet, "/users", h.list, auth.UsersManage)
	g.Handle(http.MethodPost, "/users/invite", h.invite, auth.UsersManage)
	g.Handle(http.MethodGet, "/roles", h.roles, auth.UsersManage)
	g.Handle(http.MethodPut, "/users/:id/roles", h.setRoles, auth.UsersManage)
}

func (h *Handler) me(c *gin.Context) {
	p := auth.Current(c)
	perms := make([]string, 0, len(p.Permissions))
	for name := range p.Permissions {
		perms = append(perms, name)
	}
	sort.Strings(perms)
	roles := p.Roles
	if roles == nil {
		roles = []string{}
	}
	httpx.OK(c, http.StatusOK, gin.H{"id": p.UserID, "email": p.Email, "roles": roles, "permissions": perms})
}

func (h *Handler) list(c *gin.Context) {
	p := httpx.ParsePage(c)
	ctx := c.Request.Context()
	var total int64
	if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := h.pool.Query(ctx, `
		SELECT u.id::text, u.email, u.display_name, u.is_active, u.last_login_at,
		       COALESCE(array_agg(DISTINCT r.name) FILTER (WHERE r.name IS NOT NULL), '{}')
		FROM users u LEFT JOIN user_roles ur ON ur.user_id = u.id LEFT JOIN roles r ON r.id = ur.role_id
		GROUP BY u.id ORDER BY u.email, u.id LIMIT $1 OFFSET $2`, p.Size, p.Offset())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (User, error) {
		var u User
		return u, r.Scan(&u.ID, &u.Email, &u.DisplayName, &u.IsActive, &u.LastLoginAt, &u.Roles)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}

// invite audits and sends in one transaction: a failed invitation leaves no record.
func (h *Handler) invite(c *gin.Context) {
	if h.inviter == nil {
		httpx.Fail(c, httpx.NewError(http.StatusServiceUnavailable, "INVITES_UNAVAILABLE", "Invitations aren't configured on this server."))
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, httpx.ErrBadRequest)
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if parsed, err := mail.ParseAddress(email); err != nil || parsed.Address != email {
		httpx.Fail(c, httpx.Validation("email", "Enter a valid email address."))
		return
	}
	ctx := c.Request.Context()
	err := db.InTx(ctx, h.pool, func(tx pgx.Tx) error {
		if err := audit.Write(ctx, tx, audit.Entry{ActorID: auth.Current(c).UserID, Action: "user.invite", ObjectType: "user", New: gin.H{"email": email}}); err != nil {
			return err
		}
		return h.inviter.Invite(ctx, email)
	})
	if errors.Is(err, auth.ErrAlreadyRegistered) {
		httpx.Fail(c, httpx.NewError(http.StatusConflict, "ALREADY_EXISTS", "This person already has an account."))
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
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

// setRoles replaces a user's unscoped roles. Nobody edits their own roles, which
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
		if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1::uuid`, target); err != nil {
			return err
		}
		for _, role := range req.RoleIDs {
			if _, err := tx.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) VALUES ($1::uuid, $2::uuid)`, target, role); err != nil {
				return err
			}
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
