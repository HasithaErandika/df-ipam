# DF-IPAM

Dialog Finance IP Address Management: the central platform for managing, discovering, auditing and governing IPv4 and IPv6 address infrastructure.

**Status:** Phases 0 to 2 complete, 3 to 7 built and awaiting checks against real systems. See [docs/development_phase.md](docs/development_phase.md).

| Folder | Contents |
| --- | --- |
| [`api/`](api/README.md) | Go backend: API server, worker, discovery agent, migrations |
| [`web/`](web/README.md) | React frontend |
| [`docs/`](docs/README.md) | Requirements, architecture, database, security and deployment docs |

## Requirements

- Go 1.26.10+
- Node 24+ and pnpm
- Docker with the Compose plugin, for PostgreSQL, the backend and the web app
- A Supabase project, which provides sign-in. Its database is for testing only

## Getting started

```bash
make setup       # install dependencies, create env files
make env-check   # list any missing settings
make docker-up   # start PostgreSQL, migrate, seed, start the backend and web app
make seed-demo   # optional: load a sample network, for development only
make web         # run the web app with instant reloads
```

`make docker-logs` follows the logs, and `make docker-down` stops everything. For a company install, see [docs/setup.md](docs/setup.md).

The web app runs at <http://localhost:5173> with `make web`, and in its container at <http://localhost:8088>. The API is at <http://127.0.0.1:8080>.

Three containers run: `web` (Nginx), `backend` (the API and worker in one process) and `db`. A `migrate` step runs first and exits.

## Configuration

| File | Holds |
| --- | --- |
| `api/.env` | Backend settings, including the secret keys. See [api/.env.example](api/.env.example) |
| `web/.env` | Supabase URL, anon key and API URL. See [web/.env.example](web/.env.example) |

Both are ignored by git. The Supabase `service_role` key goes in `api/.env` only, never in `web/`.

Users start with no roles. Assign the first administrator in the local database with `make db-shell`: see [docs/authentication.md](docs/authentication.md).

## Contributing

Changes go through a pull request using the [template](.github/pull_request_template.md). The `CI passed` check covers formatting, tests, builds, dependency scans and a secret scan.

## Common commands

Run `make help` for the full list.

| Command | Purpose |
| --- | --- |
| `make dev` | Start the Docker backend, then the web app |
| `make test` | Run all tests |
| `make test-db` | Run the database and API tests on a fresh `dfipam_test` database |
| `make lint` | Check Go formatting, `go vet` and web types |
| `make build` | Build the binaries and web app |
| `make migrate-up` / `migrate-down` | Apply or roll back migrations |
| `make vuln` | Scan dependencies for known vulnerabilities |
