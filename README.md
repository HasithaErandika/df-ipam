# DF-Atlas

Dialog Finance's network and IP address management platform, version 1.0.0: network discovery, the official IP register (IPAM), server monitoring and alerts, network links and FortiGate evidence, for one building on IPv4. Read-only towards the network. See [docs/README.md](docs/README.md).

| Folder | Contents |
| --- | --- |
| [`api/`](api/README.md) | Go backend: the API with the built-in scanner, migrations, seed |
| [`web/`](web/README.md) | React frontend |
| [`docs/`](docs/README.md) | Specification, setup, database, security and runbooks |

## Requirements

- Docker with the Compose plugin
- Go 1.26.11+, and Node 24+ with pnpm, for tests and commands outside Docker
- An Entra ID app registration for sign-in

## Getting started

```bash
make setup       # install dependencies, create api/.env and web/.env
make env-check   # list any missing settings
make docker-up   # start PostgreSQL, migrate, seed, start the backend and web app
make web         # run the web app with instant reloads
```

| Address | What |
| --- | --- |
| <http://localhost:5173> | The web app with `make web` |
| <http://localhost:8088> | The web app in its container |
| <http://127.0.0.1:8080> | The API |

Three containers run: `web` (Nginx), `backend` (the API, scanner and server checks in one process) and `db` (PostgreSQL). A `migrate` step runs first and exits. To make yourself an Admin and to deploy to a server, see [docs/setup.md](docs/operations/setup.md).

## Configuration

| File | Holds |
| --- | --- |
| `api/.env` | Backend settings and secrets. See [api/.env.example](api/.env.example) |
| `web/.env` | The API URL (empty: same address). See [web/.env.example](web/.env.example) |

Both are ignored by git. Secrets go in `api/.env` only.

## Common commands

Run `make help` for the full list.

| Command | Purpose |
| --- | --- |
| `make dev` | Start the containers, then the web app |
| `make lint test` | Formatting, `go vet`, web types, and all unit tests |
| `make test-db` | Database and API tests on a throwaway `dfipam_test` database |
| `make admin EMAIL=…` | Make someone an Administrator |
| `make db-shell` | `psql` as the owner |
| `make db-backup` | Encrypted backup. Needs `BACKUP_GPG_RECIPIENT` |
| `make vuln` | Scan dependencies for known vulnerabilities |

Changes go through a pull request using the [template](.github/pull_request_template.md).
