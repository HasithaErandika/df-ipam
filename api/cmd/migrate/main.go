// Command migrate applies the embedded SQL migrations using the owner role.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"dfipam/api/migrations"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 {
		return errors.New("usage: migrate up|down|version")
	}
	url := os.Getenv("MIGRATION_DATABASE_URL")
	if url == "" {
		return errors.New("MIGRATION_DATABASE_URL is not set")
	}
	_, rest, ok := strings.Cut(url, "://")
	if !ok {
		return errors.New("MIGRATION_DATABASE_URL is not a URL")
	}
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", source, "pgx5://"+rest)
	if err != nil {
		return err
	}
	defer m.Close()

	switch os.Args[1] {
	case "up":
		err = m.Up()
	case "down":
		err = m.Steps(-1)
	case "version":
		version, dirty, verr := m.Version()
		if errors.Is(verr, migrate.ErrNilVersion) {
			fmt.Println("no migrations applied")
			return nil
		}
		if verr != nil {
			return verr
		}
		fmt.Printf("version %d, dirty %v\n", version, dirty)
		return nil
	default:
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
	if errors.Is(err, migrate.ErrNoChange) {
		fmt.Println("already up to date")
		return nil
	}
	if err == nil {
		fmt.Println("done")
	}
	return err
}
