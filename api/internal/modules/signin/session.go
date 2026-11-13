package signin

import (
	"errors"
	"net/http"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

var errDisabled = errors.New("account disabled")

// startMicrosoft finds the person by their Microsoft object ID, or takes over
// the pending account for their email, or adds them, and starts a session. A
// new person has no role: an administrator gives one.
func (h *Handler) startMicrosoft(c *gin.Context, u auth.EntraUser) (string, error) {
	var token string
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var id string
		var active bool
		err := tx.QueryRow(c, `UPDATE users SET auth_subject = $2, email = $3, last_login_at = now(),
				display_name = CASE WHEN display_name = '' THEN $4 ELSE display_name END
			WHERE auth_provider = $1 AND auth_subject = $5 RETURNING id::text, is_active`,
			auth.ProviderEntra, u.ObjectID, u.Email, u.Name, auth.PendingSubject(u.Email)).Scan(&id, &active)
		if errors.Is(err, pgx.ErrNoRows) {
			err = tx.QueryRow(c, `INSERT INTO users (email, display_name, auth_provider, auth_subject, last_login_at)
				VALUES ($1, $2, $3, $4, now())
				ON CONFLICT (auth_provider, auth_subject) DO UPDATE SET email = EXCLUDED.email, last_login_at = now(),
					display_name = CASE WHEN users.display_name = '' THEN EXCLUDED.display_name ELSE users.display_name END
				RETURNING id::text, is_active`, u.Email, u.Name, auth.ProviderEntra, u.ObjectID).Scan(&id, &active)
		}
		if err != nil {
			return err
		}
		if !active {
			return errDisabled
		}
		token, err = h.openSession(c, tx, id, "microsoft", u.Email)
		return err
	})
	return token, err
}

func (h *Handler) openSession(c *gin.Context, tx pgx.Tx, userID, provider, email string) (string, error) {
	token, err := h.sessions.Create(c, tx, userID, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		return "", err
	}
	return token, audit.Write(c, tx, audit.Entry{ActorID: userID, Action: "login", ObjectType: "session",
		New: map[string]string{"provider": provider, "email": email}})
}

// csrfOK checks the header every cookie-authenticated change must carry.
func csrfOK(c *gin.Context) bool {
	if c.GetHeader(auth.CSRFHeader) != "DF-IPAM" {
		httpx.Fail(c, httpx.NewError(http.StatusForbidden, "CSRF", "This request is missing its safety header."))
		return false
	}
	return true
}
