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
	// Scoped says a role here is limited to business services, which are listed below.
	Scoped             bool     `json:"scoped"`
	BusinessServiceIDs []string `json:"businessServiceIds"`
	BusinessServices   []string `json:"businessServices"`
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
	g.Authenticated(http.MethodGet, "/auth/me/profile", h.profile)
	g.Handle(http.MethodGet, "/users", h.list, auth.UsersManage)
	g.Handle(http.MethodGet, "/people", h.people, auth.InventoryView, auth.IPRequest)
	g.Handle(http.MethodPost, "/users/invite", h.invite, auth.UsersManage)
	g.Handle(http.MethodGet, "/roles", h.roles, auth.UsersManage)
	g.Handle(http.MethodPut, "/users/:id/roles", h.setRoles, auth.UsersManage)
	g.Handle(http.MethodPut, "/users/:id/scope", h.setScope, auth.UsersManage)
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
	q := strings.TrimSpace(c.Query("q"))
	var total int64
	if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM users u WHERE $1 = '' OR u.email ILIKE '%' || $1 || '%' OR u.display_name ILIKE '%' || $1 || '%'`, q).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := h.pool.Query(ctx, `
		SELECT u.id::text, u.email, u.display_name, u.is_active, u.last_login_at,
		       COALESCE(array_agg(DISTINCT r.name) FILTER (WHERE r.name IS NOT NULL), '{}'),
		       EXISTS (SELECT 1 FROM user_roles x `+scopedRole+` WHERE x.user_id = u.id),
		       COALESCE(array_agg(DISTINCT bs.id::text) FILTER (WHERE bs.id IS NOT NULL), '{}'),
		       COALESCE(array_agg(DISTINCT bs.name) FILTER (WHERE bs.id IS NOT NULL), '{}')
		FROM users u LEFT JOIN user_roles ur ON ur.user_id = u.id LEFT JOIN roles r ON r.id = ur.role_id
		LEFT JOIN business_services bs ON bs.id = ur.business_service_id
		WHERE $3 = '' OR u.email ILIKE '%' || $3 || '%' OR u.display_name ILIKE '%' || $3 || '%'
		GROUP BY u.id ORDER BY u.email, u.id LIMIT $1 OFFSET $2`, p.Size, p.Offset(), q)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (User, error) {
		var u User
		return u, r.Scan(&u.ID, &u.Email, &u.DisplayName, &u.IsActive, &u.LastLoginAt, &u.Roles, &u.Scoped, &u.BusinessServiceIDs, &u.BusinessServices)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, out, p, total)
}

// Person is the little a picker needs: no roles, no sign-in details.
type Person struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
}

// people lists active users for owner pickers. It reveals less than /users,
// so anyone who can view inventory or make requests may use it.
func (h *Handler) people(c *gin.Context) {
	p := httpx.ParsePage(c)
	q := strings.TrimSpace(c.Query("q"))
	ctx := c.Request.Context()
	const where = ` FROM users WHERE is_active AND ($1 = '' OR email ILIKE '%' || $1 || '%' OR display_name ILIKE '%' || $1 || '%')`
	var total int64
	if err := h.pool.QueryRow(ctx, "SELECT count(*)"+where, q).Scan(&total); err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := h.pool.Query(ctx, "SELECT id::text, email, display_name"+where+" ORDER BY email LIMIT $2 OFFSET $3", q, p.Size, p.Offset())
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Person, error) {
		var u Person
		return u, r.Scan(&u.ID, &u.Email, &u.DisplayName)
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
