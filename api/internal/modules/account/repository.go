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
	AuthProvider string     `json:"authProvider"`
	IsActive     bool       `json:"isActive"`
	LastLoginAt  *time.Time `json:"lastLoginAt"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

func (r repository) profile(ctx context.Context, userID string) (Profile, error) {
	var p Profile
	err := r.conn.QueryRow(ctx, `SELECT id, email, display_name, role, auth_provider, is_active, last_login_at, created_at, updated_at
		FROM users WHERE id = $1::uuid`, userID).Scan(&p.ID, &p.Email, &p.DisplayName, &p.Role, &p.AuthProvider,
		&p.IsActive, &p.LastLoginAt, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, httpx.ErrNotFound
	}
	return p, err
}
