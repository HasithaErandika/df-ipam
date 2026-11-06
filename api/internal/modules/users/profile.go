package users

import (
	"net/http"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// RoleAssignment is one role held by a user. Scope is empty when the role applies everywhere.
type RoleAssignment struct {
	Role        string `json:"role"`
	Description string `json:"description"`
	Scope       string `json:"scope"`
}

// Profile is what a signed-in user may see about their own account.
// The provider's subject identifier stays out: it is of no use to the person.
type Profile struct {
	ID           string           `json:"id"`
	Email        string           `json:"email"`
	DisplayName  string           `json:"displayName"`
	AuthProvider string           `json:"authProvider"`
	IsActive     bool             `json:"isActive"`
	LastLoginAt  *time.Time       `json:"lastLoginAt"`
	CreatedAt    time.Time        `json:"createdAt"`
	UpdatedAt    time.Time        `json:"updatedAt"`
	Assignments  []RoleAssignment `json:"assignments"`
}

// profile only ever reads the caller's own row, so it needs no permission.
func (h *Handler) profile(c *gin.Context) {
	ctx := c.Request.Context()
	userID := auth.Current(c).UserID
	var p Profile
	err := h.pool.QueryRow(ctx, `
		SELECT id::text, email, display_name, auth_provider, is_active, last_login_at, created_at, updated_at
		FROM users WHERE id = $1::uuid`, userID).
		Scan(&p.ID, &p.Email, &p.DisplayName, &p.AuthProvider, &p.IsActive, &p.LastLoginAt, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	rows, err := h.pool.Query(ctx, `
		SELECT r.name, r.description, concat_ws(' / ', o.name, bu.name, s.name, bs.name)
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		LEFT JOIN organisations o ON o.id = ur.organisation_id
		LEFT JOIN business_units bu ON bu.id = ur.business_unit_id
		LEFT JOIN sites s ON s.id = ur.site_id
		LEFT JOIN business_services bs ON bs.id = ur.business_service_id
		WHERE ur.user_id = $1::uuid ORDER BY r.name, 3`, userID)
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	p.Assignments, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (RoleAssignment, error) {
		var a RoleAssignment
		return a, r.Scan(&a.Role, &a.Description, &a.Scope)
	})
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	httpx.OK(c, http.StatusOK, p)
}
