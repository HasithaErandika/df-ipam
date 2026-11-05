// Command agent runs approved network discovery inside a network segment. It
// pulls jobs from the API with its own credential and never connects to the
// database. Configure it with AGENT_API_URL and, on first run,
// AGENT_REGISTRATION_TOKEN from the Discovery Centre.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"dfipam/api/internal/agent"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg, err := agent.ConfigFromEnv()
	if err != nil {
		slog.Error("agent not started", "error", err)
		os.Exit(1)
	}
	if err := agent.New(cfg).Run(ctx); err != nil {
		slog.Error("agent stopped", "error", err)
		os.Exit(1)
	}
}
