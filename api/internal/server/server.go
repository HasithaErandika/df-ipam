// Package server wires middleware and routes.
package server

import (
	"context"
	"net/http"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/modules/admin"
	"dfipam/api/internal/modules/allocation"
	"dfipam/api/internal/modules/auditlog"
	"dfipam/api/internal/modules/catalog"
	"dfipam/api/internal/modules/dashboard"
	"dfipam/api/internal/modules/devices"
	"dfipam/api/internal/modules/discovery"
	"dfipam/api/internal/modules/imports"
	"dfipam/api/internal/modules/integrations"
	"dfipam/api/internal/modules/ipaddresses"
	"dfipam/api/internal/modules/notifications"
	"dfipam/api/internal/modules/organisations"
	"dfipam/api/internal/modules/reconciliation"
	"dfipam/api/internal/modules/reports"
	"dfipam/api/internal/modules/routingdomains"
	"dfipam/api/internal/modules/search"
	"dfipam/api/internal/modules/subnets"
	"dfipam/api/internal/modules/users"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/platform/safehttp"
	"dfipam/api/internal/platform/secrets"

	"github.com/gin-gonic/gin"
)

const defaultMaxInFlight = 200

type Deps struct {
	Conn     db.Conn
	Ready    func(context.Context) error
	Verifier auth.Verifier
	Resolver auth.PrincipalResolver
	Inviter  users.Inviter
	Origins  []string
	// TrustedProxies may set the client address through forwarding headers. Nginx
	// reaches the API container from the Docker gateway, not from localhost.
	TrustedProxies []string
	// MaxInFlight caps requests running at once. Zero means the default.
	MaxInFlight int
	// Keys may be nil: the API then refuses to store integration credentials.
	Keys     *secrets.Keyring
	Outbound safehttp.Policy
}

// changeTargets lists the records a change request can act on, and what each module
// allows. Fields are derived from the owning module's list.
func changeTargets() map[string]reconciliation.Target {
	names := func(fields []db.Field) map[string]bool {
		out := map[string]bool{}
		for _, f := range fields {
			out[f.Name] = true
		}
		return out
	}
	return map[string]reconciliation.Target{
		"ip_address": {Fields: names(ipaddresses.ChangeFields), Apply: ipaddresses.ApplyChange},
		"subnet":     {Fields: names(subnets.ChangeFields), Apply: subnets.ApplyChange},
		"device":     {Fields: names(devices.ChangeFields), Apply: devices.ApplyChange},
	}
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
		httpx.BodyLimit(1<<20), httpx.RateLimit(50, 100), httpx.MaxInFlight(inFlight))
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
	users.NewHandler(d.Conn, d.Inviter).Register(guard)
	organisations.NewHandler(d.Conn).Register(guard)
	routingdomains.NewHandler(d.Conn).Register(guard)
	subnets.NewHandler(subnets.NewService(d.Conn)).Register(guard)
	dashboard.NewHandler(d.Conn).Register(guard)
	auditlog.NewHandler(d.Conn).Register(guard)
	catalog.NewHandler(d.Conn).Register(guard)
	ipaddresses.NewHandler(d.Conn).Register(guard)
	reconciliation.NewHandler(d.Conn, changeTargets()).Register(guard)
	allocation.NewHandler(allocation.NewService(d.Conn)).Register(guard)
	notifications.NewHandler(d.Conn).Register(guard)
	search.NewHandler(d.Conn).Register(guard)
	imports.NewHandler(d.Conn).Register(guard)
	devices.NewHandler(d.Conn).Register(guard)
	integrations.NewHandler(integrations.NewService(d.Conn, d.Keys, d.Outbound)).Register(guard)
	reports.NewHandler(d.Conn).Register(guard)
	admin.NewHandler(d.Conn, d.Keys != nil).Register(guard)
	disc := discovery.NewHandler(discovery.NewService(d.Conn))
	disc.Register(guard)
	// Agents use their own credential, checked inside the discovery module.
	for key := range disc.RegisterAgentAPI(r.Group("/api/v1/agent-api")) {
		guard.Registered[key] = true
	}
	return r, guard
}
