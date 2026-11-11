// Package server wires middleware and routes.
package server

import (
	"context"
	"net/http"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/account"
	"dfipam/api/internal/modules/inventory"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

const defaultMaxInFlight = 200

type Deps struct {
	Conn     db.Conn
	Ready    func(context.Context) error
	Verifier auth.Verifier
	Resolver auth.PrincipalResolver
	Origins  []string
	// TrustedProxies may set the client address through forwarding headers. Nginx
	// reaches the API container from the Docker gateway, not from localhost.
	TrustedProxies []string
	// MaxInFlight caps requests running at once. Zero means the default.
	MaxInFlight int
}

// New returns the router and the guard holding every protected route.
func New(d Deps) (*gin.Engine, *auth.Guard) {
	r := gin.New()
	proxies := d.TrustedProxies
	if len(proxies) == 0 {
		proxies = []string{"127.0.0.1", "::1"}
	}
	_ = r.SetTrustedProxies(proxies)
	inFlight := d.MaxInFlight
	if inFlight <= 0 {
		inFlight = defaultMaxInFlight
	}
	r.Use(httpx.Correlation(), httpx.Logger(), httpx.Recovery(), httpx.CORS(d.Origins), httpx.SecurityHeaders(),
		httpx.BodyLimit(1<<20, nil), httpx.RateLimit(50, 100), httpx.MaxInFlight(inFlight))
	r.NoRoute(func(c *gin.Context) { httpx.Fail(c, httpx.ErrNotFound) })

	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := d.Ready(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	// A stolen token used from many addresses still counts as one person.
	perUser := httpx.RateLimitBy(func(c *gin.Context) string { return auth.Current(c).UserID }, 30, 60)
	v1 := r.Group("/api/v1", auth.Authenticate(d.Verifier, d.Resolver), perUser)
	guard := auth.NewGuard(v1)
	account.NewHandler(d.Conn).Register(guard)
	inventory.NewHandler(d.Conn).Register(guard)
	return r, guard
}
