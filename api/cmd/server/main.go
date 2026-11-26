// Command server is the DF-IPAM Lite backend: the HTTP API, the built-in scanner,
// server checks, alert rules and the email outbox, in one process (docs/engineering/architecture.md).
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/integrations/fortigate"
	"dfipam/api/internal/integrations/intune"
	"dfipam/api/internal/modules/links"
	"dfipam/api/internal/modules/notifications"
	"dfipam/api/internal/modules/signin"
	"dfipam/api/internal/platform/config"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/mail"
	"dfipam/api/internal/platform/secrets"
	"dfipam/api/internal/scanner"
	"dfipam/api/internal/server"

	"github.com/gin-gonic/gin"
)

// shutdownWait stays under the 30 seconds Docker gives before it kills the process.
const shutdownWait = 25 * time.Second

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	scan := scanner.New(pool)
	go scan.Run(ctx)
	mailCfg, mailEnabled, err := mail.FromEnv()
	if err != nil {
		return err
	}
	var mailer mail.Mailer
	if mailEnabled {
		mailer = mail.NewSMTP(mailCfg)
	}
	go notifications.RunOutbox(ctx, pool, mailer, cfg.AppBaseURL)
	go notifications.RunReservationReminders(ctx, pool)
	go links.RunReminders(ctx, pool)
	// Network link attachments live on their own volume, never in the database.
	linkFiles, err := links.OpenStore(cfg.AttachmentsDir)
	if err != nil {
		slog.Warn("network link attachments are unavailable", "reason", err.Error())
		linkFiles = nil
	}

	// The FortiGate token is stored encrypted, so the integration needs the master key.
	keys, err := secrets.FromEnv()
	if err != nil {
		slog.Warn("stored credentials are unavailable; the FortiGate can't be connected", "reason", err.Error())
		keys = nil
	}
	firewall := fortigate.NewSyncer(pool, keys)
	go firewall.Run(ctx)
	// Intune is read with the same Entra app that signs people in.
	directory := intune.NewSyncer(pool, intune.NewClient(cfg.EntraAuthority, cfg.EntraTenantID, cfg.EntraClientID, cfg.EntraClientSecret))
	go directory.Run(ctx)

	sessions := &auth.Sessions{Conn: pool}
	var microsoft signin.Microsoft
	if entra := (auth.EntraConfig{Authority: cfg.EntraAuthority, TenantID: cfg.EntraTenantID, ClientID: cfg.EntraClientID,
		ClientSecret: cfg.EntraClientSecret, RedirectURL: cfg.EntraRedirectURL, Domains: cfg.AllowedEmailDomains}); entra.Enabled() {
		microsoft = auth.NewEntra(entra)
		slog.Info("microsoft sign-in is on", "tenant", cfg.EntraTenantID, "domains", cfg.AllowedEmailDomains, "redirect", cfg.EntraRedirectURL)
	}
	signIn := signin.New(pool, microsoft, sessions, cfg.AppBaseURL)
	if microsoft == nil {
		slog.Warn("nobody can sign in: set ENTRA_TENANT_ID, ENTRA_CLIENT_ID and ENTRA_CLIENT_SECRET")
	}
	deps := server.Deps{Sessions: sessions, SignIn: signIn, Conn: pool, Ready: pool.Ping, Origins: cfg.AllowedOrigins,
		TrustedProxies: cfg.TrustedProxies, SecureCookies: strings.HasPrefix(cfg.AppBaseURL, "https://"), MaxInFlight: cfg.MaxInFlight, Scanner: scan, FortiGate: firewall, Intune: directory, LinkFiles: linkFiles, SMTPConfigured: mailEnabled}
	router, _ := server.New(deps)

	// API_LISTEN_ADDR may list several addresses, for host networking: loopback
	// for local tools, and the Docker gateway for the web container.
	errs := make(chan error, 4)
	servers := listen(cfg.ListenAddr, router, errs)
	select {
	case err := <-errs:
		stop()
		return err
	case <-ctx.Done():
	}
	return shutdownAll(servers)
}

// listen starts one server for each address in the comma-separated list. A
// server that stops sends its error to errs.
func listen(list string, router http.Handler, errs chan<- error) []*http.Server {
	var servers []*http.Server
	for _, addr := range strings.Split(list, ",") {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		srv := &http.Server{Addr: addr, Handler: router, ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 64 << 10}
		servers = append(servers, srv)
		go func() {
			slog.Info("server listening", "addr", addr)
			errs <- srv.ListenAndServe()
		}()
	}
	return servers
}

// shutdownAll lets in-flight requests finish, within shutdownWait, and
// returns the last error any server gave.
func shutdownAll(servers []*http.Server) error {
	shutdown, cancel := context.WithTimeout(context.Background(), shutdownWait)
	defer cancel()
	var failed error
	for _, srv := range servers {
		if err := srv.Shutdown(shutdown); err != nil {
			failed = err
		}
	}
	return failed
}
