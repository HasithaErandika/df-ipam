// Package account answers who the signed-in user is and what they may do.
package account

import (
	"net/http"
	"sort"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

type Handler struct{ repo repository }

func NewHandler(conn db.Conn) *Handler { return &Handler{repo: repository{conn: conn}} }

func (h *Handler) Register(g *auth.Guard) {
	g.Authenticated(http.MethodGet, "/auth/me", h.me)
	g.Authenticated(http.MethodGet, "/auth/me/profile", h.profile)
}

// Me is the signed-in user. Role is null until an Admin gives one. The session
// timeout is how long the web app waits without activity before signing out.
type Me struct {
	ID                    string   `json:"id"`
	Email                 string   `json:"email"`
	Role                  *string  `json:"role"`
	RoleName              string   `json:"roleName"`
	Permissions           []string `json:"permissions"`
	SessionTimeoutMinutes int      `json:"sessionTimeoutMinutes"`
}

func (h *Handler) me(c *gin.Context) {
	p := auth.Current(c)
	out := Me{ID: p.UserID, Email: p.Email, RoleName: p.RoleName, Permissions: make([]string, 0, len(p.Permissions))}
	if p.Role != "" {
		out.Role = &p.Role
	}
	for name := range p.Permissions {
		out.Permissions = append(out.Permissions, name)
	}
	sort.Strings(out.Permissions)
	out.SessionTimeoutMinutes = h.repo.sessionTimeout(c.Request.Context())
	c.JSON(http.StatusOK, out)
}

func (h *Handler) profile(c *gin.Context) {
	out, err := h.repo.profile(c.Request.Context(), auth.Current(c).UserID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
