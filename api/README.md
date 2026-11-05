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
| `cmd/retention` | Prunes data older than its retention policy |
| `cmd/rotate-secrets` | Re-encrypts integration credentials on the current key |
| `cmd/loadtest` | Read-only load test of the API |

## Layout

```text
api/
├── cmd/                  # entry points
├── internal/
│   ├── agent/            # discovery agent: collectors, traffic limits
│   ├── platform/         # config, database, HTTP, mail, safe outbound HTTP, secrets
│   ├── auth/             # token verification, users, permissions
│   ├── audit/            # audit log writer
│   ├── dbrules/          # tests of the database's own rules
│   ├── retention/        # retention pruning
│   ├── netcalc/          # IPv4 and IPv6 calculations
│   ├── seed/             # seed data
│   ├── server/           # route registration
│   └── modules/          # one folder per domain module
├── migrations/           # SQL, embedded into the binaries
└── openapi.yaml          # API contract
```

Modules separate HTTP handling, business rules and SQL, in files split by topic and kept under 200 lines. A module calls another module's service, never its repository. Small modules with no business rules can skip the service layer. Reference data shares one definition-driven handler in `modules/catalog`.

`make test-db` runs the schema rules, plus a full API flow as the `dfipam_app` role, on local PostgreSQL inside a transaction that is rolled back. `make test-db-supabase` runs the same on Supabase.

## Database

The real database is PostgreSQL in Docker, through `docker-compose.yml`. Supabase provides sign-in, and its database is for testing only.

Migrations run as `postgres` (`MIGRATION_DATABASE_URL`), and the app connects as `dfipam_app` (`DATABASE_URL`), which can't change audit logs or run DDL. The app uses the session pooler, where its username is `dfipam_app.<project-ref>`. The Supabase keys can't replace these, because they can't open a Postgres connection.

In Docker, `docker/postgres/init` creates `dfipam_app` with its password. On Supabase, the first migration creates it without login. Enable it once in the SQL editor, using your own password:

```sql
ALTER ROLE dfipam_app WITH LOGIN PASSWORD '<password>';
```

Supabase's Data API exposes the `public` schema to anyone with the anon key. Migration `000006` closes this with row level security and revoked grants, so data is only reachable through the Go API. `make test-db` checks this and the other schema rules, rolling back everything it does.

`sql/supabase-setup.sql` is the whole schema and seed as one file, for setting up a database from the Supabase SQL Editor. Regenerate it with `make sql`.

To add a migration, create the next numbered `.up.sql` and `.down.sql` pair in `migrations/`. Never edit a migration that has already been applied.

See [docs/database.md](../docs/database.md) for the schema and rules.
