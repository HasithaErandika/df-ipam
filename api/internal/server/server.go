// Package server wires middleware and routes.
package server

import (
	"context"
	"net/http"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/allocation"
	"dfipam/api/internal/modules/auditlog"
	"dfipam/api/internal/modules/catalog"
	"dfipam/api/internal/modules/dashboard"
	"dfipam/api/internal/modules/ipaddresses"
	"dfipam/api/internal/modules/notifications"
	"dfipam/api/internal/modules/organisations"
	"dfipam/api/internal/modules/reconciliation"
	"dfipam/api/internal/modules/routingdomains"
	"dfipam/api/internal/modules/subnets"
	"dfipam/api/internal/modules/users"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

type Deps struct {
	Conn     db.Conn
	Ready    func(context.Context) error
	Verifier auth.Verifier
	Resolver auth.PrincipalResolver
	Inviter  users.Inviter
	Origins  []string
}

// New returns the router and the guard holding every protected route.
func New(d Deps) (*gin.Engine, *auth.Guard) {
	r := gin.New()
	_ = r.SetTrustedProxies([]string{"127.0.0.1", "::1"})
	r.Use(httpx.Correlation(), httpx.Logger(), httpx.Recovery(), httpx.CORS(d.Origins), httpx.SecurityHeaders(),
		httpx.BodyLimit(1<<20), httpx.RateLimit(50, 100))
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

	v1 := r.Group("/api/v1", auth.Authenticate(d.Verifier, d.Resolver))
	guard := auth.NewGuard(v1)
	users.NewHandler(d.Conn, d.Inviter).Register(guard)
	organisations.NewHandler(d.Conn).Register(guard)
	routingdomains.NewHandler(d.Conn).Register(guard)
	subnets.NewHandler(subnets.NewService(d.Conn)).Register(guard)
	dashboard.NewHandler(d.Conn).Register(guard)
	auditlog.NewHandler(d.Conn).Register(guard)
	catalog.NewHandler(d.Conn).Register(guard)
	ipaddresses.NewHandler(d.Conn).Register(guard)
	reconciliation.NewHandler(d.Conn).Register(guard)
	allocation.NewHandler(allocation.NewService(d.Conn)).Register(guard)
	notifications.NewHandler(d.Conn).Register(guard)
	return r, guard
}
