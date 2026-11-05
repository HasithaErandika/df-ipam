// Command worker runs background work. For now that is a scheduled reconciliation
// run. Discovery, integrations and notifications arrive in later phases.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"dfipam/api/internal/modules/reconciliation"
	"dfipam/api/internal/platform/db"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := db.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("worker stopped", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	run := func() {
		out, err := reconciliation.Run(ctx, pool, "")
		if err != nil {
			slog.Error("reconciliation failed", "error", err)
			return
		}
		slog.Info("reconciliation finished", "detected", out.Detected, "resolved", out.Resolved)
	}
	run()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
