package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"dfipam/api/internal/platform/db"

	"github.com/jackc/pgx/v5"
)

// Session cookie names. On HTTPS the __Host- prefix makes the browser refuse
// one set by any other site or subdomain; plain HTTP is for local use only.
const (
	SessionCookieSecure = "__Host-dfipam_session"
	SessionCookiePlain  = "dfipam_session"
	SessionLifetime     = 10 * time.Hour
)

// SessionCookie is the cookie name for a site served over HTTPS or not.
func SessionCookie(secure bool) string {
	if secure {
		return SessionCookieSecure
	}
	return SessionCookiePlain
}

// ErrNoSession means the cookie names no live session.
var ErrNoSession = errors.New("no session")

// Sessions stores and looks up sign-in sessions.
type Sessions struct{ Conn db.Conn }

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// Create starts a session for a user inside tx, and returns the token for the
// cookie. Old, ended sessions are cleared on the way.
func (s *Sessions) Create(ctx context.Context, tx pgx.Tx, userID, ip, userAgent string) (string, error) {
	token := random() + random()
	if len(userAgent) > 300 {
		userAgent = userAgent[:300]
	}
	if _, err := tx.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now() - interval '1 day' OR revoked_at < now() - interval '1 day'`); err != nil {
		return "", err
	}
	_, err := tx.Exec(ctx, `INSERT INTO sessions (user_id, token_hash, expires_at, ip, user_agent)
		VALUES ($1::uuid, $2, now() + $3::interval, NULLIF($4, '')::inet, $5)`,
		userID, hashToken(token), SessionLifetime.String(), ip, userAgent)
	return token, err
}

// Resolve finds the signed-in user for a session token. The session must be
// unrevoked, unexpired, used within the idle timeout, and its user active.
func (s *Sessions) Resolve(ctx context.Context, token string) (*Principal, error) {
	var id, userID, email string
	var role *string
	var perms []string
	var full bool
	var roleName string
	var stale bool
	err := s.Conn.QueryRow(ctx, `SELECT s.id::text, u.id::text, u.email, u.role, COALESCE(r.permissions, '{}'), COALESCE(r.full_access, false),
			COALESCE(r.name, ''), s.last_seen_at < now() - interval '1 minute'
		FROM sessions s JOIN users u ON u.id = s.user_id LEFT JOIN roles r ON r.key = u.role
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > now() AND u.is_active
		  AND s.last_seen_at > now() - make_interval(mins => COALESCE((SELECT (value #>> '{}')::integer FROM settings WHERE key = 'session.idle_timeout_minutes'), 30))`,
		hashToken(token)).Scan(&id, &userID, &email, &role, &perms, &full, &roleName, &stale)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, err
	}
	if stale { // at most one write a minute per session
		_, _ = s.Conn.Exec(ctx, "UPDATE sessions SET last_seen_at = now() WHERE id = $1::uuid", id)
	}
	name := ""
	if role != nil {
		name = *role
	}
	p := NewPrincipal(userID, email, name, perms, full)
	p.RoleName = roleName
	return p, nil
}

// Revoke ends a session, returning its user if there was one.
func (s *Sessions) Revoke(ctx context.Context, tx pgx.Tx, token string) (string, error) {
	var userID string
	err := tx.QueryRow(ctx, `UPDATE sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL RETURNING user_id::text`,
		hashToken(token)).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return userID, err
}
