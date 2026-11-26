package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

const principalKey = "principal"

// SessionResolver finds the user behind a session cookie.
type SessionResolver interface {
	Resolve(ctx context.Context, token string) (*Principal, error)
}

// CSRFHeader must accompany every change made with a session cookie. Another
// site can't add it without a CORS preflight this API refuses.
const CSRFHeader = "X-Requested-With"

// Authenticate signs a request in by its session cookie. The user is loaded on
// every request, so deactivating someone takes effect immediately. On HTTPS
// only the __Host- cookie counts: a plain one may have been planted by another
// host under the same domain.
func Authenticate(sessions SessionResolver, secure bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := SessionToken(c, secure)
		if token == "" || sessions == nil {
			httpx.Fail(c, httpx.ErrUnauthenticated)
			return
		}
		bySession(c, sessions, token)
	}
}

// SessionToken reads the session cookie this site issues: the __Host- one on
// HTTPS, the plain one otherwise.
func SessionToken(c *gin.Context, secure bool) string {
	v, _ := c.Cookie(SessionCookie(secure))
	return v
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
