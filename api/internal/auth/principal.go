package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrInactive means the account exists but has been deactivated.
var ErrInactive = errors.New("account inactive")

// Principal is the authenticated user for one request. Role is empty until an
// Admin gives one.
type Principal struct {
	UserID      string
	Email       string
	Role        string
	Permissions map[string]bool
}

func (p *Principal) Has(permission string) bool { return p.Permissions[permission] }

// NewPrincipal builds a principal with the permissions of its role.
func NewPrincipal(userID, email, role string) *Principal {
	p := &Principal{UserID: userID, Email: email, Role: role, Permissions: map[string]bool{}}
	for _, name := range RolePermissions[role] {
		p.Permissions[name] = true
	}
	return p
}

// Resolver maps a verified identity to a DF-IPAM user and their permissions.
type Resolver struct {
	Pool     *pgxpool.Pool
	Provider string
}

func (r *Resolver) Resolve(ctx context.Context, id Identity) (*Principal, error) {
	var userID string
	var role *string
	var active bool
	// Read first: this runs on every request and should not write each time.
	err := r.Pool.QueryRow(ctx, `SELECT id, role, is_active FROM users WHERE auth_provider = $1 AND auth_subject = $2`,
		r.Provider, id.Subject).Scan(&userID, &role, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		err = r.Pool.QueryRow(ctx, `
			INSERT INTO users (email, auth_provider, auth_subject, last_login_at)
			VALUES ($1, $2, $3, now())
			ON CONFLICT (auth_provider, auth_subject) DO UPDATE SET last_login_at = now()
			RETURNING id, role, is_active`, id.Email, r.Provider, id.Subject).Scan(&userID, &role, &active)
	}
	if err != nil {
		return nil, fmt.Errorf("provision user: %w", err)
	}
	if !active {
		return nil, ErrInactive
	}
	name := ""
	if role != nil {
		name = *role
	}
	return NewPrincipal(userID, id.Email, name), nil
}
