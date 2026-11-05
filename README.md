# DF-IPAM

Dialog Finance IP Address Management: the central platform for managing, discovering, auditing and governing IPv4 and IPv6 address infrastructure.

**Status:** foundation complete. See [docs/development_phase.md](docs/development_phase.md).

| Folder | Contents |
| --- | --- |
| [`api/`](api/README.md) | Go backend: API server, worker, discovery agent, migrations |
| [`web/`](web/README.md) | React frontend |
| [`docs/`](docs/README.md) | Requirements, architecture, database, security and deployment docs |

## Requirements

- Go 1.26.10+
- Node 24+ and pnpm
- A Supabase project, which provides both the database and sign-in

## Getting started

```bash
make setup       # install dependencies, create env files
make env-check   # list any missing settings
make db-setup    # apply migrations and seed roles to Supabase
make dev         # run the API and the web app
```

The web app runs at <http://localhost:5173> and the API at <http://127.0.0.1:8080>.

## Configuration

| File | Holds |
| --- | --- |
| `api/.env` | Backend settings, including the secret keys. See [api/.env.example](api/.env.example) |
| `web/.env` | Supabase URL, anon key and API URL. See [web/.env.example](web/.env.example) |

Both are ignored by git. The Supabase `service_role` key goes in `api/.env` only, never in `web/`.

Users start with no roles. Roles are managed in the Supabase database: see [docs/authentication.md](docs/authentication.md).

## Contributing

Changes go through a pull request using the [template](.github/pull_request_template.md). The `CI passed` check covers formatting, tests, builds, dependency scans and a secret scan.

## Common commands

Run `make help` for the full list.

| Command | Purpose |
| --- | --- |
| `make dev` | Run the API and the web app |
| `make test` | Run all tests |
| `make lint` | Check formatting, types and lint rules |
| `make build` | Build the binaries and web app |
| `make migrate-up` / `migrate-down` | Apply or roll back migrations |
| `make vuln` | Scan dependencies for known vulnerabilities |
