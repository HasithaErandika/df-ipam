# Deployment

## Topology

One dedicated on-premises Ubuntu Server. Agents run on separate hosts in the network segments they cover.

| Component | Runs as |
| --- | --- |
| Nginx | Serves the React build, HTTPS termination, proxies `/api` |
| `df-ipam-api` | Docker container, published on localhost only |
| `df-ipam-worker` | Docker container |
| `df-ipam-agent` | systemd service on each agent host, `CAP_NET_RAW` only |
| PostgreSQL | Docker container with a named volume, published on localhost only |

Each service has its own account with minimal file permissions. Nothing runs as root. The supplement's PHP-FPM, Laravel and Python services are replaced by the Go services above: see [requirements.md](requirements.md).

## Network flows

| From | To | Port | Purpose |
| --- | --- | --- | --- |
| Users | Nginx | 443/tcp | Web app and API |
| Users | Supabase | 443/tcp | Sign-in |
| Agents | Nginx | 443/tcp | Jobs, results, heartbeats |
| API | Supabase | 443/tcp | JWKS |
| API, worker | PostgreSQL | 5432/tcp | Database, on Docker's private network |
| Worker | FortiGate, vCenter | 443/tcp (or the configured admin port) | Read-only APIs |
| Worker | AWS, Azure | 443/tcp | Read-only APIs |
| Worker | Mail provider | 443/tcp (Graph) or 587/tcp (SMTP) | Notifications |
| Agents | Approved targets | ICMP, 161/udp, 53/udp+tcp | Discovery |

All other traffic is blocked. PostgreSQL is never reachable from untrusted networks.

## Agent privileges

| Function | Needs |
| --- | --- |
| ICMP | `CAP_NET_RAW` |
| Nmap host discovery | `CAP_NET_RAW` and `CAP_NET_ADMIN`, in a separate unit if used, launched with `--privileged` |
| Reading local ARP or neighbour tables, DNS, SNMP | No special capability |

Capabilities are set only on the collector's systemd unit, with `AmbientCapabilities`, a matching `CapabilityBoundingSet` and `NoNewPrivileges=yes`. Other services get none. Exact Nmap needs are confirmed on the target Ubuntu version in Phase 4.

## Sizing

Starting estimates, **not measurements**. Final sizing needs the real figures (see [requirements.md](requirements.md), open item 9) and a load test.

| Size | Inventory | Agents | App server | Self-hosted PostgreSQL |
| --- | --- | --- | --- | --- |
| Small | Under 50k IPs | Up to 5 | 4 vCPU, 8 GB | 4 vCPU, 16 GB, 250 GB SSD |
| Medium | Up to 300k IPs | Up to 20 | 8 vCPU, 16 GB | 8 vCPU, 32 GB, 1 TB SSD |
| Large | Over 300k IPs | Over 20 | Measure first | Measure first |

Observations drive storage. As a rule of thumb: IPs × runs per day × sources × about 500 bytes × retention days. For example, 300,000 IPs × 4 runs × 2 sources × 500 bytes is about 1.2 GB a day, before indexes. Retention is the main control.

## Outbound access

Needed: Supabase auth (users' browsers and the API), the Supabase database, mail provider, and the integration endpoints. All others are blocked.

## Database

PostgreSQL 16 runs in Docker on the server, with its data in the `df-ipam_db-data` volume. The network inventory never leaves the premises. Start with [setup.md](setup.md).

Connections stay on Docker's private network without TLS, because they never leave the host. Move PostgreSQL to its own server and TLS with certificate verification becomes required.

Supabase provides sign-in. Its database is for testing only, through `make test-db-supabase` and `make migrate-supabase`. Its session pooler allows 15 clients in total, and the Data API must stay closed: see migration `000006` and [security.md](security.md).

## Configuration

Settings live in `api/.env` and `web/.env`, in development and on the server. `make` loads `api/.env` and passes values to the containers by name. On the server it has mode 600.

| Variable | Purpose |
| --- | --- |
| `POSTGRES_PASSWORD`, `DFIPAM_APP_PASSWORD` | Local database passwords. Letters and digits only |
| `POSTGRES_DB`, `POSTGRES_PORT`, `API_PORT` | Database name, and host ports on localhost |
| `DATABASE_URL`, `MIGRATION_DATABASE_URL` | Connections for commands run on the host. Containers build their own |
| `SUPABASE_DATABASE_URL`, `SUPABASE_MIGRATION_DATABASE_URL` | Supabase database, for testing only |
| `DB_MAX_CONNS` | Connection pool size for processes run on the host |
| `TRUSTED_PROXIES`, `API_MAX_IN_FLIGHT` | Who may state the client's address, and the most requests running at once |
| `DOCKER_SUBNET`, `DOCKER_GATEWAY` | The Docker network. The API trusts only this gateway |
| `INTEGRATION_ALLOWED_HOSTS`, `INTEGRATION_ALLOWED_PRIVATE_RANGES`, `INTEGRATION_CA_FILE` | Outbound policy for connectors |
| `SECRET_ENCRYPTION_KEY_VERSION`, `SECRET_ENCRYPTION_KEY_PREVIOUS` | Key rotation |
| `BACKUP_GPG_RECIPIENT` | GPG key backups are encrypted to |
| `REDIS_URL` | Queue |
| `SUPABASE_URL`, `SUPABASE_ANON_KEY`, `SUPABASE_SERVICE_ROLE_KEY` | Sign-in checks and invitations |
| `CORS_ALLOWED_ORIGINS` | Web origins allowed to call the API |
| `SECRET_ENCRYPTION_KEY` | Master key for integration credentials (Phase 5) |
| `APP_BASE_URL` | Public URL of the web app, used for links in emails |
| `SMTP_HOST`, `SMTP_PORT` | Mail server for the worker. Empty host means email is off. Port 465 uses TLS; other ports must offer STARTTLS |
| `SMTP_USERNAME`, `SMTP_PASSWORD` | Mail server sign-in, if it needs one. Secret |
| `MAIL_FROM` | Sender address for notifications |

## Database roles

| Role | Rights |
| --- | --- |
| `postgres` | Owns schema, runs migrations. Not used at runtime |
| `dfipam_app` | Read/write on app tables, `INSERT`/`SELECT` only on `audit_logs` |
| `dfipam_readonly` | Read-only access for support and reporting |

## Release process

1. Run tests and vulnerability scans in CI.
2. Back up the database.
3. Run `make docker-up`, which rebuilds the image, migrates, seeds and restarts what changed.
4. Check `/ready` and watch the logs.

Migrations are forward-compatible, so the previous binary keeps working if a rollback is needed.

## Backup and recovery

- **Daily:** `make db-backup` writes an encrypted `pg_dump`, on a systemd timer. **Copy it off the server**, because the database and backups otherwise share one machine.
- **For a shorter RPO:** add continuous WAL archiving (`pgBackRest`). This isn't set up.
- **Also back up:** `api/.env` and the encryption key, stored **separately** from the database backups. Without the key, stored integration credentials can't be recovered.
- **Verify:** check integrity after every backup. A successful backup command doesn't prove a backup can be restored.
- **Test:** do a full restore at least quarterly and record the result.
- **Retention:** to be agreed with compliance.

### RPO and RTO

Examples for business approval, not organisational requirements.

| Option | RPO (data loss) | RTO (time to restore) | Needs |
| --- | --- | --- | --- |
| Standard | 24 hours | 8 hours | Daily backups |
| Recommended starting point | 15 minutes | 4 hours | WAL archiving, tested runbook |
| High | Under 1 minute | 1 hour | Standby server |

### Recovery runbooks

See [runbooks.md](runbooks.md). They haven't been rehearsed yet.

## Monitoring

- `/health` (process up) and `/ready` (database reachable)
- Structured JSON logs to journald, forwarded to the central log platform
- Alerts: service down, queue backlog, agents offline, sync failures, backup failure, certificate expiry
