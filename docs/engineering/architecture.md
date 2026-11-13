# Architecture

## Containers

| Container | Holds | Reached through |
| --- | --- | --- |
| `web` | Nginx: the built web app, TLS, rate limits, and the way in to the API | The browser |
| `backend` | One Go process: the API, the scanner, server checks, the FortiGate reader and the email outbox | `web` only |
| `db` | PostgreSQL 16, volume `df-ipam-lite-db-data` | `backend` only |

A `migrate` step applies migrations and the seed, then exits. People sign in with Microsoft Entra ID (or the development sign-in); all data stays in PostgreSQL.

Instead of the `web` container, Apache on the host can serve the app and proxy the API: `deploy/apache/df-ipam.conf`, `docker-compose.apache.yml`, `make apache-up` and `make apache-web` (see [setup.md](../operations/setup.md#run-it-behind-apache)).

Compose files: `docker-compose.yml` (always), `docker-compose.dev.yml` (local ports), `docker-compose.prod.yml` (publishes 80 and 443 with TLS), `docker-compose.apache.yml` (backend on localhost for Apache), and `docker-compose.hostnet.yml` (with `HOST_NETWORK=true`: the backend on the host's network, to read MAC addresses). Always start them with `make`, which loads the env files.

## Background loops

All inside the backend, cancelled cleanly on shutdown.

| Loop | Runs | Does |
| --- | --- | --- |
| Discovery | Every `scan.interval_minutes` (default 10), or on Scan now | Sweeps each segment, saves sightings, evaluates alerts, removes sightings unseen for 90 days and alerts closed for a year |
| Server checks | Every `server_check.interval_seconds` (default 5 minutes) | Pings monitored entries twice, or their check port; records all results in one update; evaluates alerts |
| FortiGate | Every `fortigate.interval_minutes`, when switched on | Reads ARP, DHCP leases and VPN sessions; evaluates alerts |
| Email outbox | Every 5 seconds | Sends queued emails when email is on and SMTP is set, at most 20 an hour per person; hourly, removes read notifications older than 90 days |
| Reservation reminders | Every 6 hours | Queues one reminder per reservation in the week before it expires |

Alert evaluation takes a database advisory lock, so two loops never evaluate at once. See [discovery.md](../product/discovery.md) for what the scanner sends and its limits.

## Backend (`api/`)

Go with Gin, pgx and golang-migrate.

```text
api/
├── cmd/
│   ├── server/        # the API and background loops
│   ├── migrate/       # up, down, version
│   ├── seed/          # environments and default settings
│   ├── sqlbundle/     # prints the whole schema and seed as one SQL file
│   └── ouigen/        # rebuilds the built-in MAC maker list
├── internal/
│   ├── auth/          # token verification, users, permissions, route guard
│   ├── audit/         # audit writer
│   ├── scanner/       # discovery and server checks
│   ├── integrations/fortigate/   # read-only FortiGate client
│   ├── modules/       # account, admin, alerts, allocationlite, auditlog, discovery,
│   │                  # integrations, inventory, monitoring, notifications, reports
│   ├── ipam/          # Available candidate and "in use" rules
│   ├── netcalc/       # IPv4 maths
│   ├── notify/        # notification helper
│   ├── platform/      # config, db, httpx, mail, secrets, tabular (CSV and Excel)
│   ├── platforms/     # platform and service codes
│   ├── validate/      # input checks
│   ├── seed/  sqlbundle/  dbrules/  server/
├── migrations/        # SQL, embedded in the binaries
└── openapi.yaml
```

- HTTP in handlers, rules in services, SQL in repositories. Modules call each other's services only.
- Files stay under 200 lines, and functions at cognitive complexity 15 or less.
- `monitoring` evaluates the alert rules. The scanner, the FortiGate reader and inventory changes call it.

## Frontend (`web/`)

React, TypeScript, Vite, Tailwind CSS, React Router and TanStack Query. See [frontend.md](frontend.md).

```text
web/src/
├── app/           # router (lazy screens), navigation, shell, query client
├── components/    # ui/ design system, platform icons, subnet map, charts
├── features/      # one folder per screen
├── lib/           # API client, auth (the only sign-in file), formats, types
└── index.css      # colour tokens
```

## Tests

| Command | Covers |
| --- | --- |
| `make test` | Go unit tests and web tests |
| `make test-db` | The schema rules and every API route, as `dfipam_app`, on a throwaway `dfipam_test` database |
| `make lint` | `gofmt`, `go vet`, and web type checks |
