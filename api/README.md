# DF-IPAM API

The Go backend: Gin, pgx and golang-migrate. Run commands from the repository root with `make` (see `make help`).

## Binaries

| Path | Purpose |
| --- | --- |
| `cmd/api` | HTTP server under `/api/v1` |
| `cmd/worker` | Background jobs |
| `cmd/agent` | Network discovery agent |
| `cmd/migrate` | `up`, `down` and `version` for database migrations |
| `cmd/seed` | Loads roles, permissions and default settings |
| `cmd/sqlbundle` | Prints the full setup SQL |

## Layout

```text
api/
├── cmd/                  # entry points
├── internal/
│   ├── platform/         # config, database, HTTP helpers
│   ├── auth/             # token verification, users, permissions
│   ├── audit/            # audit log writer
│   ├── netcalc/          # IPv4 and IPv6 calculations
│   ├── seed/             # seed data
│   ├── server/           # route registration
│   └── modules/          # one folder per domain module
├── migrations/           # SQL, embedded into the binaries
└── openapi.yaml          # API contract
```

Modules separate HTTP handling, business rules and SQL. A module calls another module's service, never its repository. Small modules with no business rules can skip the service layer. Reference data shares one definition-driven handler in `modules/catalog`.

`make test-db` runs the schema rules, plus a full API flow as the `dfipam_app` role, on the real database inside a transaction that is rolled back.

## Database

The database is Supabase PostgreSQL. There is no local database.

Migrations run as `postgres` (`MIGRATION_DATABASE_URL`), and the app connects as `dfipam_app` (`DATABASE_URL`), which can't change audit logs or run DDL. The app uses the session pooler, where its username is `dfipam_app.<project-ref>`. The Supabase keys can't replace these, because they can't open a Postgres connection.

The first migration creates `dfipam_app` without login. Enable it once in the Supabase SQL editor, using your own password:

```sql
ALTER ROLE dfipam_app WITH LOGIN PASSWORD '<password>';
```

Supabase's Data API exposes the `public` schema to anyone with the anon key. Migration `000006` closes this with row level security and revoked grants, so data is only reachable through the Go API. `make test-db` checks this and the other schema rules, rolling back everything it does.

`sql/supabase-setup.sql` is the whole schema and seed as one file, for setting up a database from the Supabase SQL Editor. Regenerate it with `make sql`.

To add a migration, create the next numbered `.up.sql` and `.down.sql` pair in `migrations/`. Never edit a migration that has already been applied.

See [docs/database.md](../docs/database.md) for the schema and rules.
