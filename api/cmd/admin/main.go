// Command admin makes a person an Administrator by their work email, using the
// owner role: `make admin EMAIL=name@dialogfinance.lk`. Someone who has signed
// in already is promoted; anyone else is added as a pending Microsoft account
// that their first sign-in takes over. Audited.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"dfipam/api/internal/audit"
	"dfipam/api/internal/auth"
	"dfipam/api/internal/platform/db"

	"github.com/jackc/pgx/v5"
)

func main() {
	email := flag.String("email", "", "work email of the new Administrator")
	flag.Parse()
	if err := run(strings.ToLower(strings.TrimSpace(*email))); err != nil {
		fmt.Fprintln(os.Stderr, "admin:", err)
		os.Exit(1)
	}
}

func run(email string) error {
	if _, domain, ok := strings.Cut(email, "@"); !ok || domain == "" {
		return errors.New("give a work email: make admin EMAIL=name@dialogfinance.lk")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, os.Getenv("MIGRATION_DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	return db.InTx(ctx, pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `UPDATE users SET role = 'admin', is_active = true WHERE lower(email) = $1 RETURNING id::text`, email)
		if err != nil {
			return err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		action := "grant_admin"
		if len(ids) == 0 {
			id, err := auth.AddPendingAdmin(ctx, tx, email)
			if err != nil {
				return err
			}
			ids, action = []string{id}, "create"
		}
		for _, id := range ids {
			if err := audit.Write(ctx, tx, audit.Entry{ActorType: "system", Action: action, ObjectType: "user", ObjectID: id,
				New: map[string]string{"email": email, "role": "admin", "via": "make admin"}}); err != nil {
				return err
			}
		}
		fmt.Printf("%s is an Administrator (%s)\n", email, map[string]string{"create": "added; takes effect at first sign-in", "grant_admin": "existing account"}[action])
		return nil
	})
}
