// Command agent will run approved network discovery. It deliberately performs no
// scanning yet: discovery arrives in Phase 4, together with its safety controls.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	slog.Info("agent idle: discovery is not implemented yet")
	<-ctx.Done()
}
