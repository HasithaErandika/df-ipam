# Deployment

## Topology

One dedicated on-premises Ubuntu Server. Agents run on separate hosts in the network segments they cover.

| Component | Runs as |
| --- | --- |
| Nginx | Serves the React build, HTTPS termination, proxies `/api` |
| `df-ipam-api` | systemd service, bound to localhost |
| `df-ipam-worker` | systemd service |
| `df-ipam-agent` | systemd service on each agent host, `CAP_NET_RAW` only |
| Redis | Local, bound to localhost, password protected, pending approval |
| PostgreSQL | Supabase |

Each service has its own account with minimal file permissions. Nothing runs as root. The supplement's PHP-FPM, Laravel and Python services are replaced by the Go services above: see [requirements.md](requirements.md).

## Network flows

| From | To | Port | Purpose |
| --- | --- | --- | --- |
| Users | Nginx | 443/tcp | Web app and API |
| Users | Supabase | 443/tcp | Sign-in |
| Agents | Nginx | 443/tcp | Jobs, results, heartbeats |
| API | Supabase | 443/tcp | JWKS |
| API, worker | PostgreSQL | 5432/tcp (TLS) | Database. Local if self-hosted |
| API, worker | Redis | 6379/tcp | Localhost only |
| Worker | FortiGate, vCenter | 443/tcp (or the configured admin port) | Read-only APIs |
| Worker | AWS, Azure | 443/tcp | Read-only APIs |
| Worker | Mail provider | 443/tcp (Graph) or 587/tcp (SMTP) | Notifications |
| Agents | Approved targets | ICMP, 161/udp, 53/udp+tcp | Discovery |

All other traffic is blocked. PostgreSQL and Redis are never reachable from untrusted networks.

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

## Database: Supabase

Supabase PostgreSQL is the only database for now, and there is no local one. The application uses standard PostgreSQL, so moving later is a change of connection string plus a data migration.

- The project is in `ap-northeast-1`. The API uses the **session pooler** (`aws-0-ap-northeast-1.pooler.supabase.com`), which works over IPv4. The direct `db.<ref>.supabase.co` host used for migrations is IPv6 only.
- The Data API must stay closed to DF-IPAM tables. See migration `000006` and [security.md](security.md).

**Before production data is loaded,** Dialog Finance must confirm:

- [ ] Is a full network inventory allowed outside the premises and country?
- [ ] Does the Supabase region satisfy residency rules?
- [ ] Do the plan's availability and backup window meet the requirement?
- [ ] Is it acceptable that the app can't work during an internet outage?
- [ ] Is a vendor security review needed?

A system holding a bank's network map, and needed during network incidents, may well need an on-premises database in the end.

## Configuration

In development, settings live in `api/.env` and `web/.env`. On the server, environment files live in `/etc/df-ipam/`, owned by root and readable only by the service user.

| Variable | Purpose |
| --- | --- |
| `DATABASE_URL` | App role connection |
| `MIGRATION_DATABASE_URL` | Owner role, used only for migrations |
| `REDIS_URL` | Queue |
| `SUPABASE_URL`, `SUPABASE_ANON_KEY`, `SUPABASE_SERVICE_ROLE_KEY` | Sign-in checks and invitations |
| `CORS_ALLOWED_ORIGINS` | Web origins allowed to call the API |
| `SECRET_ENCRYPTION_KEY` | Master key for integration credentials (Phase 5) |
| `APP_BASE_URL` | Public URL, used in emails and CORS |

## Database roles

| Role | Rights |
| --- | --- |
| `postgres` | Owns schema, runs migrations. Not used at runtime |
| `dfipam_app` | Read/write on app tables, `INSERT`/`SELECT` only on `audit_logs` |
| `dfipam_readonly` | Read-only access for support and reporting |

## Release process

1. Build binaries and the frontend in CI, running tests and vulnerability scans.
2. Back up the database.
3. Run migrations with the owner role.
4. Replace binaries, then restart the worker and API.
5. Check `/health` and watch the logs.

Migrations are forward-compatible, so the previous binary keeps working if a rollback is needed.

## Backup and recovery

- **Self-hosted:** daily base backup with continuous WAL archiving (`pgBackRest`), encrypted, with a copy off the server.
- **Supabase:** provider backups, plus a scheduled encrypted `pg_dump` kept on-prem.
- **Also back up:** `/etc/df-ipam/` and the encryption key, stored **separately** from the database backups. Without the key, stored integration credentials can't be recovered.
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

To be written and rehearsed in Phase 7:

- Restore the database to a point in time
- Rebuild the app server from backups
- Recover or rotate the encryption key and integration credentials
- Replace a lost agent, and re-register agents
- Recover from Redis loss: jobs are re-queued from their Postgres records

## Monitoring

- `/health` (process up) and `/ready` (database and Redis reachable)
- Structured JSON logs to journald, forwarded to the central log platform
- Alerts: service down, queue backlog, agents offline, sync failures, backup failure, certificate expiry
