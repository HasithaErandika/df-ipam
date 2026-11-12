package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

const principalKey = "principal"

// PrincipalResolver is the part of Resolver the middleware needs.
type PrincipalResolver interface {
	Resolve(ctx context.Context, id Identity) (*Principal, error)
}

// SessionResolver finds the user behind a session cookie (Microsoft sign-in).
type SessionResolver interface {
	Resolve(ctx context.Context, token string) (*Principal, error)
}

// CSRFHeader must accompany every change made with a session cookie. Another
// site can't add it without a CORS preflight this API refuses.
const CSRFHeader = "X-Requested-With"

// Authenticate accepts a Supabase bearer token or, when sessions is set, a
// Microsoft sign-in session cookie. The user is loaded on every request, so
// deactivating someone takes effect immediately.
func Authenticate(v Verifier, r PrincipalResolver, sessions SessionResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" && sessions != nil {
			if token := sessionToken(c); token != "" {
				bySession(c, sessions, token)
				return
			}
		}
		byBearer(c, v, r)
	}
}

func sessionToken(c *gin.Context) string {
	for _, name := range []string{SessionCookieSecure, SessionCookiePlain} {
		if v, err := c.Cookie(name); err == nil && v != "" {
			return v
		}
	}
	return ""
}

func bySession(c *gin.Context, sessions SessionResolver, token string) {
	switch c.Request.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
	default:
		if c.GetHeader(CSRFHeader) != "DF-IPAM" {
			httpx.Fail(c, httpx.NewError(http.StatusForbidden, "CSRF", "This request is missing its safety header. Reload the page and try again."))
			return
		}
	}
	p, err := sessions.Resolve(c.Request.Context(), token)
	if errors.Is(err, ErrNoSession) {
		httpx.Fail(c, httpx.ErrUnauthenticated)
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Set(principalKey, p)
	c.Next()
}

func byBearer(c *gin.Context, v Verifier, r PrincipalResolver) {
	scheme, token, ok := strings.Cut(c.GetHeader("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		httpx.Fail(c, httpx.ErrUnauthenticated)
		return
	}
	id, err := v.Verify(c.Request.Context(), token)
	if err != nil {
		if !errors.Is(err, ErrInvalidToken) {
			slog.Error("token verification unavailable", "error", err)
			httpx.Fail(c, httpx.NewError(http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Sign-in checks are unavailable. Try again shortly."))
			return
		}
		httpx.Fail(c, httpx.ErrUnauthenticated)
		return
	}
	p, err := r.Resolve(c.Request.Context(), id)
	if errors.Is(err, ErrInactive) {
		httpx.Fail(c, httpx.NewError(http.StatusForbidden, "ACCOUNT_DISABLED", "This account has been deactivated."))
		return
	}
	if err != nil {
		httpx.Fail(c, err)
		return
	}
	c.Set(principalKey, p)
	c.Next()
}

// Current returns the authenticated user for this request.
func Current(c *gin.Context) *Principal {
	p, _ := c.MustGet(principalKey).(*Principal)
	return p
}

func require(permissions []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := Current(c)
		for _, name := range permissions {
			if p.Has(name) {
				c.Next()
				return
			}
		}
		httpx.Fail(c, httpx.ErrForbidden)
	}
}

// Guard registers routes so that none can exist without a stated permission.
type Guard struct {
	group      *gin.RouterGroup
	Registered map[string]bool
}

func NewGuard(group *gin.RouterGroup) *Guard {
	return &Guard{group: group, Registered: map[string]bool{}}
}

func (g *Guard) record(method, path string) {
	g.Registered[method+" "+strings.TrimRight(g.group.BasePath(), "/")+path] = true
}

// Handle adds a route the caller may use with any one of the permissions.
func (g *Guard) Handle(method, path string, h gin.HandlerFunc, anyOf ...string) {
	if len(anyOf) == 0 {
		panic("route needs a permission: " + method + " " + path)
	}
	g.record(method, path)
	g.group.Handle(method, path, require(anyOf), h)
}

// Authenticated adds a route open to any signed-in user, such as /auth/me.
func (g *Guard) Authenticated(method, path string, h gin.HandlerFunc) {
	g.record(method, path)
	g.group.Handle(method, path, h)
}
