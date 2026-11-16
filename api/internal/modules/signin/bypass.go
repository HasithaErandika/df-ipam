package signin

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// WithBypass switches on the development sign-in for one work email. It is for
// while Microsoft Entra ID isn't set up: anyone who can open the site signs in
// as that person, without a password. Config refuses it in production.
func (h *Handler) WithBypass(email string) *Handler {
	h.bypass = strings.ToLower(strings.TrimSpace(email))
	return h
}

var errBypassOff = httpx.NewError(http.StatusNotFound, "BYPASS_OFF", "The development sign-in is off.")

// bypassSignIn starts an ordinary session for the bypass person: the same
// cookie, idle timeout and audit as a Microsoft sign-in.
func (h *Handler) bypassSignIn(c *gin.Context) {
	if h.bypass == "" {
		httpx.Fail(c, errBypassOff)
		return
	}
	if !csrfOK(c) {
		return
	}
	var token string
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		id, err := h.bypassUser(c, tx)
		if err != nil {
			return err
		}
		token, err = h.openSession(c, tx, id, "bypass", h.bypass)
		return err
	})
	if errors.Is(err, errDisabled) {
		httpx.Fail(c, httpx.NewError(http.StatusForbidden, "ACCOUNT_DISABLED", "This account has been deactivated."))
		return
	}
	if err != nil {
		slog.ErrorContext(c, "development sign-in failed", "error", err)
		httpx.Fail(c, err)
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(auth.SessionCookie(h.secure), token, int(auth.SessionLifetime.Seconds()), "/", "", h.secure, true)
	c.Status(http.StatusNoContent)
}

// bypassUser finds the account for the bypass email. Without one, it adds a
// pending Microsoft account as an Administrator, so the first person in can set
// everything up, and their later Microsoft sign-in takes the same account over.
func (h *Handler) bypassUser(c *gin.Context, tx pgx.Tx) (string, error) {
	var id string
	var active bool
	err := tx.QueryRow(c, `SELECT id::text, is_active FROM users WHERE lower(email) = $1
		ORDER BY is_active DESC, role IS NULL, created_at LIMIT 1`, h.bypass).Scan(&id, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		if id, err = auth.AddPendingAdmin(c, tx, h.bypass); err != nil {
			return "", err
		}
		active = true
		err = audit.Write(c, tx, audit.Entry{ActorType: "system", Action: "create", ObjectType: "user", ObjectID: id,
			New: map[string]string{"email": h.bypass, "role": "admin", "reason": "development sign-in"}})
	}
	if err != nil {
		return "", err
	}
	if !active {
		return "", errDisabled
	}
	_, err = tx.Exec(c, `UPDATE users SET last_login_at = now() WHERE id = $1::uuid`, id)
	return id, err
}
