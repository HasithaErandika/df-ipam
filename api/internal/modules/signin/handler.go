// Package signin runs sign-in with Microsoft Entra ID (the redirect to
// Microsoft, and the callback that checks the result and starts a session) and
// sign-out. These routes
// are public by nature: they are how a person becomes signed in.
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
	// maxAttempts bounds memory (about 2 MB). perClient stops one address from
	// filling it: a client over its share loses its own oldest attempt, never
	// someone else's.
	maxAttempts = 10000
	perClient   = 5
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
	attempts map[string]heldAttempt
}

type heldAttempt struct {
	auth.LoginAttempt
	client string
}

func New(conn db.Conn, ms Microsoft, sessions *auth.Sessions, baseURL string) *Handler {
	return &Handler{conn: conn, ms: ms, sessions: sessions, base: strings.TrimRight(baseURL, "/"),
		secure: strings.HasPrefix(baseURL, "https://"), attempts: map[string]heldAttempt{}}
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
// already holds a session.
func (h *Handler) status(c *gin.Context) {
	session := false
	if token := cookieToken(c); token != "" {
		_, err := h.sessions.Resolve(c, token)
		session = err == nil
	}
	c.JSON(http.StatusOK, gin.H{"microsoft": h.ms != nil, "session": session})
}

func (h *Handler) login(c *gin.Context) {
	if h.ms == nil {
		httpx.Fail(c, httpx.NewError(http.StatusNotFound, "SSO_OFF", "Microsoft sign-in isn't set up."))
		return
	}
	a := auth.NewAttempt(safeReturn(c.Query("returnTo")))
	h.remember(a, c.ClientIP())
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(attemptCookie, a.State, int(attemptTTL.Seconds()), "/api/v1/auth/microsoft", "", h.secure, true)
	c.Redirect(http.StatusFound, h.ms.AuthCodeURL(a))
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
	token, err := h.startMicrosoft(c, u)
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

func (h *Handler) logout(c *gin.Context) {
	if !csrfOK(c) {
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
