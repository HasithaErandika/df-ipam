package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SuperAdminRole holds every permission.
const SuperAdminRole = "Super Administrator"

// ErrInactive means the account exists but has been deactivated.
var ErrInactive = errors.New("account inactive")

// Principal is the authenticated user for one request.
type Principal struct {
	UserID      string
	Email       string
	Roles       []string
	Permissions map[string]bool
}

func (p *Principal) Has(permission string) bool { return p.Permissions[permission] }

// Resolver maps a verified identity to a DF-IPAM user and their permissions.
type Resolver struct {
	Pool     *pgxpool.Pool
	Provider string
}

func (r *Resolver) Resolve(ctx context.Context, id Identity) (*Principal, error) {
	var userID string
	var active bool
	// Read first: this runs on every request and should not write each time.
	err := r.Pool.QueryRow(ctx, `SELECT id, is_active FROM users WHERE auth_provider = $1 AND auth_subject = $2`,
		r.Provider, id.Subject).Scan(&userID, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		err = r.Pool.QueryRow(ctx, `
			INSERT INTO users (email, auth_provider, auth_subject, last_login_at)
			VALUES ($1, $2, $3, now())
			ON CONFLICT (auth_provider, auth_subject) DO UPDATE SET last_login_at = now()
			RETURNING id, is_active`, id.Email, r.Provider, id.Subject).Scan(&userID, &active)
	}
	if err != nil {
		return nil, fmt.Errorf("provision user: %w", err)
	}
	if !active {
		return nil, ErrInactive
	}
	return r.load(ctx, userID, id.Email)
}

func (r *Resolver) load(ctx context.Context, userID, email string) (*Principal, error) {
	p := &Principal{UserID: userID, Email: email, Permissions: map[string]bool{}}
	rows, err := r.Pool.Query(ctx, `SELECT r.name FROM user_roles ur JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1 GROUP BY r.name ORDER BY r.name`, userID)
	if err != nil {
		return nil, err
	}
	if p.Roles, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
		return nil, err
	}
	rows, err = r.Pool.Query(ctx, `SELECT DISTINCT pm.resource || ':' || pm.action
		FROM user_roles ur
		JOIN role_permissions rp ON rp.role_id = ur.role_id
		JOIN permissions pm ON pm.id = rp.permission_id
		WHERE ur.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	perms, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	for _, name := range perms {
		p.Permissions[name] = true
	}
	return p, nil
}
