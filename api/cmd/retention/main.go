// Command retention prunes old data, using the owner role. See deploy/systemd.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/retention"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("retention failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	url := os.Getenv("MIGRATION_DATABASE_URL")
	if url == "" {
		return fmt.Errorf("MIGRATION_DATABASE_URL is not set")
	}
	pool, err := db.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	out, err := retention.Run(ctx, pool)
	if err != nil {
		return err
	}
	slog.Info("retention finished", "removed", out)
	return nil
}
