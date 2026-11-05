// Command rotate-secrets re-encrypts integration credentials under the current key.
// Set the new key as SECRET_ENCRYPTION_KEY with its version, keep the old one in
// SECRET_ENCRYPTION_KEY_PREVIOUS, run this, then remove the old key.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"dfipam/api/internal/modules/integrations"
	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/platform/safehttp"
	"dfipam/api/internal/platform/secrets"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("rotation failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	keys, err := secrets.FromEnv()
	if err != nil {
		return err
	}
	pool, err := db.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	n, err := integrations.NewService(pool, keys, safehttp.Policy{}).RotateCredentials(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("re-encrypted %d credentials under key version %d\n", n, keys.Version())
	return nil
}
