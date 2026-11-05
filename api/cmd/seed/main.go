// Command seed loads reference data using the owner role.
package main

import (
	"context"
	"fmt"
	"os"

	"dfipam/api/internal/platform/db"
	"dfipam/api/internal/seed"

	"github.com/jackc/pgx/v5"
)

func main() {
	ctx := context.Background()
	pool, err := db.Connect(ctx, os.Getenv("MIGRATION_DATABASE_URL"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := db.InTx(ctx, pool, func(tx pgx.Tx) error { return seed.Run(ctx, tx) }); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
	fmt.Println("seeded")
}
