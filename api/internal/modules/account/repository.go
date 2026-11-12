package account

import (
	"context"
	"errors"
	"time"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/jackc/pgx/v5"
)

type repository struct{ conn db.Conn }

// Profile is the signed-in user's own account record.
type Profile struct {
	ID           string     `json:"id"`
	Email        string     `json:"email"`
	DisplayName  string     `json:"displayName"`
	Role         *string    `json:"role"`
	RoleName     *string    `json:"roleName"`
	AuthProvider string     `json:"authProvider"`
	IsActive     bool       `json:"isActive"`
	LastLoginAt  *time.Time `json:"lastLoginAt"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

func (r repository) profile(ctx context.Context, userID string) (Profile, error) {
	var p Profile
	err := r.conn.QueryRow(ctx, `SELECT u.id, u.email, u.display_name, u.role, r.name, u.auth_provider, u.is_active, u.last_login_at, u.created_at, u.updated_at
		FROM users u LEFT JOIN roles r ON r.key = u.role WHERE u.id = $1::uuid`, userID).Scan(&p.ID, &p.Email, &p.DisplayName, &p.Role, &p.RoleName, &p.AuthProvider,
		&p.IsActive, &p.LastLoginAt, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, httpx.ErrNotFound
	}
	return p, err
}

// DefaultSessionTimeout applies until an Admin sets one in Settings.
const DefaultSessionTimeout = 30

func (r repository) sessionTimeout(ctx context.Context) int {
	var n int
	err := r.conn.QueryRow(ctx, `SELECT (value #>> '{}')::integer FROM settings WHERE key = 'session.idle_timeout_minutes'`).Scan(&n)
	if err != nil || n < 5 {
		return DefaultSessionTimeout
	}
	return n
}
