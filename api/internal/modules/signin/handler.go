// Package signin runs Microsoft Entra ID sign-in: the redirect to Microsoft,
// the callback that checks the result and starts a session, and sign-out.
// These routes are public by nature: they are how a person becomes signed in.
package signin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const (
	attemptCookie = "dfipam_login"
	attemptTTL    = 10 * time.Minute
	maxAttempts   = 1000
)

// Microsoft is the part of auth.Entra this module uses.
type Microsoft interface {
	AuthCodeURL(a auth.LoginAttempt) string
	SignIn(ctx context.Context, code string, a auth.LoginAttempt) (auth.EntraUser, error)
}

type Handler struct {
	conn     db.Conn
	ms       Microsoft // nil when Microsoft sign-in isn't set up
	sessions *auth.Sessions
	base     string // APP_BASE_URL: where the app is, and where people return to
	secure   bool   // served over HTTPS, so cookies are Secure and __Host-

	mu       sync.Mutex
	attempts map[string]auth.LoginAttempt
}

func New(conn db.Conn, ms Microsoft, sessions *auth.Sessions, baseURL string) *Handler {
	return &Handler{conn: conn, ms: ms, sessions: sessions, base: strings.TrimRight(baseURL, "/"),
		secure: strings.HasPrefix(baseURL, "https://"), attempts: map[string]auth.LoginAttempt{}}
}

// Paths are the public routes, for the route guard test.
var Paths = []string{"GET /api/v1/auth/status", "GET /api/v1/auth/microsoft/login", "GET /api/v1/auth/microsoft/callback", "POST /api/v1/auth/logout"}

func (h *Handler) Register(r gin.IRouter) {
	g := r.Group("/api/v1/auth", httpx.RateLimit(2, 20))
	g.GET("/status", h.status)
	g.GET("/microsoft/login", h.login)
	g.GET("/microsoft/callback", h.callback)
	g.POST("/logout", h.logout)
}

// status tells the sign-in page which ways in exist, and whether this browser
// already holds a Microsoft session.
func (h *Handler) status(c *gin.Context) {
	session := false
	if token := cookieToken(c); token != "" {
		_, err := h.sessions.Resolve(c, token)
		session = err == nil
	}
	c.JSON(http.StatusOK, gin.H{"microsoft": h.ms != nil, "password": true, "session": session})
}

func (h *Handler) login(c *gin.Context) {
	if h.ms == nil {
		httpx.Fail(c, httpx.NewError(http.StatusNotFound, "SSO_OFF", "Microsoft sign-in isn't set up."))
		return
	}
	a := auth.NewAttempt(safeReturn(c.Query("returnTo")))
	h.remember(a)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(attemptCookie, a.State, int(attemptTTL.Seconds()), "/api/v1/auth/microsoft", "", h.secure, true)
	c.Redirect(http.StatusFound, h.ms.AuthCodeURL(a))
}

func (h *Handler) remember(a auth.LoginAttempt) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for k, v := range h.attempts {
		if time.Since(v.Started) > attemptTTL || len(h.attempts) >= maxAttempts {
			delete(h.attempts, k)
		}
	}
	h.attempts[a.State] = a
}

// take returns the attempt for state once, and only while it is fresh.
func (h *Handler) take(state string) (auth.LoginAttempt, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	a, ok := h.attempts[state]
	delete(h.attempts, state)
	return a, ok && time.Since(a.Started) <= attemptTTL
}

// safeReturn keeps the return address inside the app.
func safeReturn(p string) string {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.HasPrefix(p, "/\\") || strings.HasPrefix(p, "/api/") {
		return "/"
	}
	return p
}

func (h *Handler) fail(c *gin.Context, reason string) {
	c.Redirect(http.StatusFound, h.base+"/?signin_error="+reason)
}

func (h *Handler) callback(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(attemptCookie, "", -1, "/api/v1/auth/microsoft", "", h.secure, true)
	if h.ms == nil {
		h.fail(c, "off")
		return
	}
	if e := c.Query("error"); e != "" {
		h.fail(c, map[bool]string{true: "cancelled", false: "failed"}[e == "access_denied"])
		return
	}
	state, _ := c.Cookie(attemptCookie)
	a, ok := h.take(c.Query("state"))
	if !ok || state == "" || state != a.State {
		h.fail(c, "expired")
		return
	}
	u, err := h.ms.SignIn(c, c.Query("code"), a)
	if err != nil {
		h.refuse(c, u, err)
		return
	}
	token, err := h.start(c, u)
	if errors.Is(err, errDisabled) {
		h.fail(c, "disabled")
		return
	}
	if err != nil {
		slog.ErrorContext(c, "microsoft sign-in could not start a session", "error", err)
		h.fail(c, "failed")
		return
	}
	c.SetCookie(auth.SessionCookie(h.secure), token, int(auth.SessionLifetime.Seconds()), "/", "", h.secure, true)
	c.Redirect(http.StatusFound, h.base+a.ReturnTo)
}

// refuse records a refused sign-in and tells the person why.
func (h *Handler) refuse(c *gin.Context, u auth.EntraUser, err error) {
	switch {
	case errors.Is(err, auth.ErrEntraDomain):
		h.audit(c, "", "login_denied", map[string]string{"email": u.Email, "reason": "domain"})
		h.fail(c, "domain")
	case errors.Is(err, auth.ErrEntraTenant):
		h.audit(c, "", "login_denied", map[string]string{"reason": "tenant"})
		h.fail(c, "tenant")
	default:
		slog.WarnContext(c, "microsoft sign-in failed", "error", err)
		h.fail(c, "failed")
	}
}

var errDisabled = errors.New("account disabled")

// start finds or adds the person by their Microsoft object ID and starts a
// session. A new person has no role: an administrator gives one.
func (h *Handler) start(c *gin.Context, u auth.EntraUser) (string, error) {
	var token string
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		var id string
		var active bool
		if err := tx.QueryRow(c, `INSERT INTO users (email, display_name, auth_provider, auth_subject, last_login_at)
			VALUES ($1, $2, $3, $4, now())
			ON CONFLICT (auth_provider, auth_subject) DO UPDATE SET email = EXCLUDED.email, last_login_at = now(),
				display_name = CASE WHEN users.display_name = '' THEN EXCLUDED.display_name ELSE users.display_name END
			RETURNING id::text, is_active`, u.Email, u.Name, auth.ProviderEntra, u.ObjectID).Scan(&id, &active); err != nil {
			return err
		}
		if !active {
			return errDisabled
		}
		var err error
		if token, err = h.sessions.Create(c, tx, id, c.ClientIP(), c.Request.UserAgent()); err != nil {
			return err
		}
		return audit.Write(c, tx, audit.Entry{ActorID: id, Action: "login", ObjectType: "session", New: map[string]string{"provider": "microsoft", "email": u.Email}})
	})
	return token, err
}

func (h *Handler) logout(c *gin.Context) {
	if c.GetHeader(auth.CSRFHeader) != "DF-IPAM" {
		httpx.Fail(c, httpx.NewError(http.StatusForbidden, "CSRF", "This request is missing its safety header."))
		return
	}
	if token := cookieToken(c); token != "" {
		err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
			id, err := h.sessions.Revoke(c, tx, token)
			if err != nil || id == "" {
				return err
			}
			return audit.Write(c, tx, audit.Entry{ActorID: id, Action: "logout", ObjectType: "session"})
		})
		if err != nil {
			httpx.Fail(c, err)
			return
		}
	}
	c.SetSameSite(http.SameSiteLaxMode)
	for _, name := range []string{auth.SessionCookieSecure, auth.SessionCookiePlain} {
		c.SetCookie(name, "", -1, "/", "", name == auth.SessionCookieSecure || h.secure, true)
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) audit(c *gin.Context, actor, action string, values any) {
	err := db.InTx(c, h.conn, func(tx pgx.Tx) error {
		return audit.Write(c, tx, audit.Entry{ActorID: actor, ActorType: map[bool]string{true: "system", false: "user"}[actor == ""],
			Action: action, ObjectType: "session", New: values})
	})
	if err != nil {
		slog.ErrorContext(c, "audit of a refused sign-in failed", "error", err)
	}
}

func cookieToken(c *gin.Context) string {
	for _, name := range []string{auth.SessionCookieSecure, auth.SessionCookiePlain} {
		if v, err := c.Cookie(name); err == nil && v != "" {
			return v
		}
	}
	return ""
}
