// Package server wires middleware and routes.
package server

import (
	"context"
	"net/http"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/integrations/fortigate"
	"dfipam/api/internal/integrations/intune"
	"dfipam/api/internal/modules/account"
	"dfipam/api/internal/modules/admin"
	"dfipam/api/internal/modules/alerts"
	"dfipam/api/internal/modules/allocationlite"
	"dfipam/api/internal/modules/auditlog"
	"dfipam/api/internal/modules/discovery"
	firewallmod "dfipam/api/internal/modules/firewall"
	"dfipam/api/internal/modules/integrations"
	"dfipam/api/internal/modules/inventory"
	"dfipam/api/internal/modules/links"
	"dfipam/api/internal/modules/notifications"
	"dfipam/api/internal/modules/reports"
	"dfipam/api/internal/modules/signin"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/httpx"
	"dfipam/api/internal/platform/mail"

	"github.com/gin-gonic/gin"
)

const defaultMaxInFlight = 200

// requestDeadline is under the server's 60 s write timeout and the proxy's 60 s
// read timeout, so a request that runs too long is rolled back, not committed
// after the person has already seen an error.
const requestDeadline = 55 * time.Second

// importPaths take files of up to 5 MB; every other route takes 1 MB. Room is
// left for the multipart framing around the file.
var importPaths = map[string]int64{
	"/api/v1/devices/import":       6 << 20,
	"/api/v1/ip-addresses/import":  6 << 20,
	"/api/v1/segments/import":      6 << 20,
	"/api/v1/network-links/import": 6 << 20,
	// Attachments of up to 15 MB, with room for the form around them.
	"/api/v1/network-links/:id/attachments": links.MaxFileBytes + 1<<20,
}

type Deps struct {
	Conn    db.Conn
	Ready   func(context.Context) error
	Origins []string
	// TrustedProxies may set the client address through forwarding headers. Nginx
	// reaches the API container from the Docker gateway, not from localhost.
	TrustedProxies []string
	// SecureCookies is true when the site is served over HTTPS: only the
	// __Host- session cookie then signs a request in.
	SecureCookies bool
	// MaxInFlight caps requests running at once. Zero means the default.
	MaxInFlight int
	// Scanner runs discovery in this process. Nil in tests: its routes then answer 503.
	Scanner discovery.Scanner
	// FortiGate reads the firewall when an Admin connects one. Nil in tests.
	FortiGate *fortigate.Syncer
	// Intune names each device's user when an Admin switches it on. Nil in tests.
	Intune *intune.Syncer
	// LinkFirewall replaces FortiGate for Network Links, in tests.
	LinkFirewall links.Firewall
	// LinkFiles keeps network link attachments. Nil answers their routes with 503.
	LinkFiles *links.Store
	// SMTPConfigured says whether api/.env has a mail server, shown in Settings.
	SMTPConfigured bool
	// Mailer sends the test email from Settings. Nil without a mail server.
	Mailer mail.Mailer
	// AppBaseURL is where links in email point.
	AppBaseURL string
	// Sessions resolves sign-in cookies, the only way in. Nil refuses every request.
	Sessions auth.SessionResolver
	// SignIn serves the public sign-in routes. Nil in tests that don't need them.
	SignIn *signin.Handler
}

// New returns the router and the guard holding every protected route.
func New(d Deps) (*gin.Engine, *auth.Guard) {
	r := gin.New()
	// c.Done() and c.Err() follow the request, so work stops when it is cancelled.
	r.ContextWithFallback = true
	proxies := d.TrustedProxies
	if len(proxies) == 0 {
		proxies = []string{"127.0.0.1", "::1"}
	}
	if err := r.SetTrustedProxies(proxies); err != nil {
		panic("trusted proxies: " + err.Error()) // config.Load checks them, so this is a programming error
	}
	inFlight := d.MaxInFlight
	if inFlight <= 0 {
		inFlight = defaultMaxInFlight
	}
	r.Use(httpx.Correlation(), httpx.Logger(), httpx.Recovery(), httpx.Deadline(requestDeadline), httpx.CORS(d.Origins), httpx.SecurityHeaders(),
		httpx.BodyLimit(1<<20, importPaths), httpx.RateLimit(50, 100), httpx.MaxInFlight(inFlight))
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
	if d.SignIn != nil {
		d.SignIn.Register(r)
	}
	v1 := r.Group("/api/v1", auth.Authenticate(d.Sessions, d.SecureCookies), perUser)
	guard := auth.NewGuard(v1)
	account.NewHandler(d.Conn).Register(guard)
	inventory.NewHandler(d.Conn).Register(guard)
	alerts.New(d.Conn).Register(guard)
	notifications.New(d.Conn, d.Mailer, d.AppBaseURL).Register(guard)
	allocationlite.New(d.Conn).Register(guard)
	discovery.New(d.Conn, d.Scanner).Register(guard)
	admin.New(d.Conn, d.SMTPConfigured).Register(guard)
	auditlog.New(d.Conn).Register(guard)
	reports.New(d.Conn).Register(guard)
	integrations.New(d.Conn, d.FortiGate).Register(guard)
	integrations.NewIntune(d.Conn, d.Intune).Register(guard)
	var firewall links.Firewall
	if d.FortiGate != nil { // a nil *Syncer must not become a non-nil interface
		firewall = d.FortiGate
	}
	if d.LinkFirewall != nil {
		firewall = d.LinkFirewall
	}
	links.New(d.Conn, d.LinkFiles, firewall).Register(guard)
	var fw firewallmod.Connector
	if firewall != nil {
		fw = firewall
	}
	firewallmod.New(d.Conn, fw).Register(guard)
	return r, guard
}
