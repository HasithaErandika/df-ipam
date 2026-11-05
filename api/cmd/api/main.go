// Command api is the DF-IPAM HTTP server.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/config"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/server"

	"github.com/gin-gonic/gin"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("api stopped", "error", err)
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

	admin := auth.NewSupabaseAdmin(cfg.SupabaseURL, cfg.SupabaseAnonKey, cfg.SupabaseServiceKey)
	if err := admin.Check(ctx); err != nil {
		slog.Warn("supabase auth is not reachable", "error", err)
	}
	verifier := auth.NewSupabaseVerifier(cfg.SupabaseURL+"/auth/v1", cfg.AuthAudience, cfg.SupabaseURL+"/auth/v1/.well-known/jwks.json")
	resolver := &auth.Resolver{Pool: pool, Provider: verifier.Provider()}
	deps := server.Deps{Conn: pool, Ready: pool.Ping, Verifier: verifier, Resolver: resolver, Origins: cfg.AllowedOrigins}
	if cfg.SupabaseServiceKey != "" {
		deps.Inviter = admin
	}
	router, _ := server.New(deps)

	srv := &http.Server{Addr: cfg.ListenAddr, Handler: router, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 2 * time.Minute}
	errs := make(chan error, 1)
	go func() {
		slog.Info("api listening", "addr", cfg.ListenAddr)
		errs <- srv.ListenAndServe()
	}()
	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}
