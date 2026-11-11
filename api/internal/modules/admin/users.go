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
	g.Handle(http.MethodGet, "/users/options", h.userOptions, auth.UsersManage, auth.SystemManage)
	g.Handle(http.MethodGet, "/environments", h.environments, auth.InventoryView, auth.IPRequest)
	g.Handle(http.MethodPost, "/environments", h.createEnvironment, auth.SystemManage)
	g.Handle(http.MethodPut, "/environments/:id", h.updateEnvironment, auth.SystemManage)
	g.Handle(http.MethodDelete, "/environments/:id", h.deleteEnvironment, auth.SystemManage)
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
	IsActive    bool       `db:"is_active" json:"isActive"`
	LastLoginAt *time.Time `db:"last_login_at" json:"lastLoginAt"`
	CreatedAt   time.Time  `db:"created_at" json:"createdAt"`
}

func (h *Handler) users(c *gin.Context) {
	var w db.Where
	switch role := c.Query("role"); role {
	case "":
	case "none":
		w.Raw("role IS NULL")
	default:
		if !auth.ValidRole(role) {
			httpx.Fail(c, httpx.Validation("role", "Choose a valid role."))
			return
		}
		w.Add("role = ?", role)
	}
	if a := c.Query("active"); a == "true" || a == "false" {
		w.Add("is_active = ?::boolean", a)
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		w.Add(`(email ILIKE ? ESCAPE '\' OR display_name ILIKE ? ESCAPE '\')`, db.Contains(q))
	}
	page := httpx.ParsePage(c)
	items, total, err := db.ListPage[userDTO](c, h.conn, `SELECT id::text AS id, email, display_name, role, is_active, last_login_at, created_at FROM users`+
		w.SQL()+" ORDER BY role IS NOT NULL, lower(email), id", "SELECT count(*) FROM users"+w.SQL(), w.Args, page)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.List(c, items, page, total)
}

// userOptions lists active people for pickers, such as a reservation's owner.
func (h *Handler) userOptions(c *gin.Context) {
	out, err := db.All[userDTO](c, h.conn, `SELECT id::text AS id, email, display_name, role, is_active, last_login_at, created_at
		FROM users WHERE is_active AND role IS NOT NULL ORDER BY lower(email) LIMIT 500`)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, out)
}

// updateUser sets a person's role, name and whether they may sign in. Nobody can
// change their own role or access, and the last active Admin can't be removed.
func (h *Handler) updateUser(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var in struct {
		Role        string `json:"role"`
		IsActive    bool   `json:"isActive"`
		DisplayName string `json:"displayName"`
	}
	if !httpx.Bind(c, &in) {
		return
	}
	var v validate.V
	if in.Role != "" && !auth.ValidRole(in.Role) {
		v.Add("role", "Choose Admin, Requester or Viewer.")
	}
	v.Text("displayName", &in.DisplayName, 120, false, "Display name")
	if err := v.Err(); err != nil {
		httpx.Fail(c, err)
		return
	}
	self := auth.Current(c).UserID == id
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var oldRole *string
		var oldActive bool
		if err := tx.QueryRow(c, "SELECT role, is_active FROM users WHERE id = $1::uuid FOR UPDATE", id).Scan(&oldRole, &oldActive); err != nil {
			return err
		}
		wasAdmin := oldRole != nil && *oldRole == auth.RoleAdmin && oldActive
		staysAdmin := in.Role == auth.RoleAdmin && in.IsActive
		if self && (!staysAdmin && wasAdmin || in.IsActive != oldActive) {
			return httpx.NewError(http.StatusConflict, "SELF_CHANGE", "You can't change your own role or access. Ask another Admin.")
		}
		if wasAdmin && !staysAdmin {
			var admins int
			if err := tx.QueryRow(c, "SELECT count(*) FROM users WHERE role = 'admin' AND is_active").Scan(&admins); err != nil {
				return err
			}
			if admins <= 1 {
				return httpx.NewError(http.StatusConflict, "LAST_ADMIN", "This is the last active Admin. Make someone else an Admin first.")
			}
		}
		if _, err := tx.Exec(c, `UPDATE users SET role = NULLIF($2, ''), is_active = $3, display_name = $4 WHERE id = $1::uuid`,
			id, in.Role, in.IsActive, in.DisplayName); err != nil {
			return err
		}
		return record(c, tx, "update", "user", id, map[string]any{"role": oldRole, "isActive": oldActive}, in)
	})
	if err != nil {
		httpx.Fail(c, db.Friendly(err))
		return
	}
	c.Status(http.StatusNoContent)
}

func actorID(c *gin.Context) string { return auth.Current(c).UserID }
