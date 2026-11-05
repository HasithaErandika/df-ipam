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

// Authenticate verifies the bearer token and loads the user on every request,
// so deactivating a user takes effect immediately.
func Authenticate(v Verifier, r PrincipalResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
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
