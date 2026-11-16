# DF-IPAM API

The Go backend: Gin, pgx and golang-migrate. Run commands from the repository root with `make` (see `make help`).

## Binaries

| Path | Purpose |
| --- | --- |
| `cmd/server` | The API under `/api/v1`, with the scanner, server checks, FortiGate reader, email outbox and reminders in the same process |
| `cmd/migrate` | `up`, `down` and `version` for migrations |
| `cmd/seed` | Loads the five environments and default settings. Safe to repeat |
| `cmd/sqlbundle` | Prints every migration and the seed as one SQL file (`make sql`) |
| `cmd/ouigen` | Rebuilds the built-in MAC maker list from the IEEE register |

## Layout

See [docs/architecture.md](../docs/engineering/architecture.md). In short: `internal/modules/<name>` for each area, `internal/scanner` for discovery, `internal/platform` for shared infrastructure, `migrations/` for SQL embedded in the binaries, and `openapi.yaml` for the contract.

- HTTP in handlers, rules in services, SQL in repositories. A module calls another module's service, never its SQL.
- Every route is registered with its permission through the route guard.
- Files under 200 lines.

## Database

PostgreSQL in Docker. Migrations run as `postgres` (`MIGRATION_DATABASE_URL`); the app connects as `dfipam_app` (`DATABASE_URL`), which can't change the audit log or run DDL.

To change the schema, add the next numbered `.up.sql` and `.down.sql` pair in `migrations/`, then run `make sql`. Never edit an applied migration. See [docs/database.md](../docs/engineering/database.md).

## Tests

`make test` runs the unit tests. `make test-db` runs the schema rules and every API route as `dfipam_app` on a throwaway `dfipam_test` database.
