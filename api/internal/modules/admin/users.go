// Package admin manages users and roles, environments and system settings.
package admin

import (
	"net/http"
	"strings"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/validate"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

type Handler struct {
	conn           db.Conn
	smtpConfigured bool
}

func New(conn db.Conn, smtpConfigured bool) *Handler {
	return &Handler{conn: conn, smtpConfigured: smtpConfigured}
}

func (h *Handler) Register(g *auth.Guard) {
	g.Handle(http.MethodGet, "/users", h.users, auth.UsersManage)
	g.Handle(http.MethodPut, "/users/:id", h.updateUser, auth.UsersManage)
	g.Handle(http.MethodGet, "/users/options", h.userOptions, auth.UsersManage, auth.SystemManage, auth.InventoryEdit)
	g.Handle(http.MethodGet, "/roles", h.roles, auth.UsersManage)
	g.Handle(http.MethodPost, "/roles", h.createRole, auth.UsersManage)
	g.Handle(http.MethodPut, "/roles/:key", h.updateRole, auth.UsersManage)
	g.Handle(http.MethodDelete, "/roles/:key", h.deleteRole, auth.UsersManage)
	g.Handle(http.MethodGet, "/environments", h.environments, auth.InventoryView, auth.IPRequest)
	g.Handle(http.MethodPost, "/environments", h.createEnvironment, auth.NetworkManage)
	g.Handle(http.MethodPut, "/environments/:id", h.updateEnvironment, auth.NetworkManage)
	g.Handle(http.MethodDelete, "/environments/:id", h.deleteEnvironment, auth.NetworkManage)
	g.Handle(http.MethodGet, "/departments", h.departments, auth.InventoryView, auth.IPRequest)
	g.Handle(http.MethodPost, "/departments", h.createDepartment, auth.NetworkManage)
	g.Handle(http.MethodPut, "/departments/:id", h.updateDepartment, auth.NetworkManage)
	g.Handle(http.MethodDelete, "/departments/:id", h.deleteDepartment, auth.NetworkManage)
	g.Handle(http.MethodGet, "/settings", h.settings, auth.SystemManage)
	g.Handle(http.MethodPut, "/settings", h.updateSettings, auth.SystemManage)
}

func record(c *gin.Context, tx pgx.Tx, action, kind, id string, old, new any) error {
	return audit.Write(c, tx, audit.Entry{ActorID: auth.Current(c).UserID, Action: action, ObjectType: kind, ObjectID: id, Old: old, New: new})
}

type userDTO struct {
	ID          string     `db:"id" json:"id"`
	Email       string     `db:"email" json:"email"`
	DisplayName string     `db:"display_name" json:"displayName"`
	Role        *string    `db:"role" json:"role"`
	RoleName    *string    `db:"role_name" json:"roleName"`
	IsActive    bool       `db:"is_active" json:"isActive"`
	LastLoginAt *time.Time `db:"last_login_at" json:"lastLoginAt"`
	CreatedAt   time.Time  `db:"created_at" json:"createdAt"`
}

func (h *Handler) users(c *gin.Context) {
	var w db.Where
	switch role := c.Query("role"); role {
	case "":
	case "none":
		w.Raw("u.role IS NULL")
	default:
		w.Add("u.role = ?", role)
	}
	if a := c.Query("active"); a == "true" || a == "false" {
		w.Add("u.is_active = ?::boolean", a)
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		w.Add(`(u.email ILIKE ? ESCAPE '\' OR u.display_name ILIKE ? ESCAPE '\')`, db.Contains(q))
	}
	page := httpx.ParsePage(c)
	items, total, err := db.ListPage[userDTO](c, h.conn, userSelect+w.SQL()+" ORDER BY u.role IS NOT NULL, lower(u.email), u.id",
		"SELECT count(*) FROM users u"+w.SQL(), w.Args, page)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, items, page, total)
}

const userSelect = `SELECT u.id::text AS id, u.email, u.display_name, u.role, r.name AS role_name, u.is_active, u.last_login_at, u.created_at
	FROM users u LEFT JOIN roles r ON r.key = u.role`

// userOptions lists active people for pickers, such as a reservation's owner.
func (h *Handler) userOptions(c *gin.Context) {
	out, err := db.All[userDTO](c, h.conn, userSelect+` WHERE u.is_active AND u.role IS NOT NULL ORDER BY lower(u.email) LIMIT 500`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

type userChange struct {
	Role        string `json:"role"`
	IsActive    bool   `json:"isActive"`
	DisplayName string `json:"displayName"`
}

// updateUser sets a person's role, name and whether they may sign in. Nobody can
// change their own role or access, nobody can give or take away more than they
// hold, and the last active person who can manage users can't lose that, so
// nobody is ever locked out.
func (h *Handler) updateUser(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var in userChange
	if !httpx.Bind(c, &in) {
		return
	}
	var v validate.V
	v.Text("displayName", &in.DisplayName, 120, false, "Display name")
	if err := v.Err(); err != nil {
		httpx.Fail(c, err)
		return
	}
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error { return applyUserChange(c, tx, id, in) })
	if err != nil {
		httpx.Fail(c, db.Friendly(err))
		return
	}
	c.Status(http.StatusNoContent)
}

func applyUserChange(c *gin.Context, tx pgx.Tx, id string, in userChange) error {
	if err := lockUserManagers(c, tx); err != nil {
		return err
	}
	var oldRole *string
	var oldActive, couldManage bool
	if err := tx.QueryRow(c, "SELECT u.role, u.is_active, u.is_active AND "+auth.HolderSQL("$2")+" FROM users u WHERE u.id = $1::uuid FOR UPDATE",
		id, auth.UsersManage).Scan(&oldRole, &oldActive, &couldManage); err != nil {
		return err
	}
	if changed := in.Role != deref(oldRole) || in.IsActive != oldActive; changed {
		if err := checkAccessChange(c, tx, id, deref(oldRole), in, couldManage); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(c, `UPDATE users SET role = NULLIF($2, ''), is_active = $3, display_name = $4 WHERE id = $1::uuid`,
		id, in.Role, in.IsActive, in.DisplayName); err != nil {
		return err
	}
	return record(c, tx, "update", "user", id, map[string]any{"role": oldRole, "isActive": oldActive}, in)
}

// checkAccessChange runs the rules for a change of role or access.
func checkAccessChange(c *gin.Context, tx pgx.Tx, id, oldRole string, in userChange, couldManage bool) error {
	if auth.Current(c).UserID == id {
		return httpx.NewError(http.StatusConflict, "SELF_CHANGE", "You can't change your own role or access. Ask another administrator.")
	}
	canManage := false
	if in.Role != "" {
		if err := tx.QueryRow(c, "SELECT $3 AND (r.full_access OR $2 = ANY(r.permissions)) FROM roles r WHERE r.key = $1",
			in.Role, auth.UsersManage, in.IsActive).Scan(&canManage); err != nil {
			return httpx.Validation("role", "Choose a role from the list.")
		}
	}
	if err := checkUserGrant(c, tx, oldRole, in.Role); err != nil {
		return err
	}
	if !couldManage || canManage {
		return nil
	}
	var others int
	if err := tx.QueryRow(c, "SELECT count(*) FROM users u WHERE u.is_active AND u.id <> $1::uuid AND "+auth.HolderSQL("$2"),
		id, auth.UsersManage).Scan(&others); err != nil {
		return err
	}
	if others == 0 {
		return httpx.NewError(http.StatusConflict, "LAST_ADMIN", "This is the last active person who can manage users. Give someone else that first.")
	}
	return nil
}

func actorID(c *gin.Context) string { return auth.Current(c).UserID }

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
