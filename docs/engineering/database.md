# Database

PostgreSQL 16 in Docker (`docker-compose.yml`, volume `df-ipam-lite-db-data`). It is the only database. Network link attachments are files on their own volume, `df-ipam-lite-link-files` (mounted at `/data/attachments` in the backend), and only their records are in the database.

## Rules

- **IPv4 only.** Addresses are `inet`, networks `cidr`. Read them back with `host()` or `network::text`: plain `inet::text` adds `/32`.
- **Schema changes** go in a new numbered `.up.sql` and `.down.sql` pair in `api/migrations/`, followed by `make sql`. Never edit an applied migration.
- **Every table** has `ENABLE ROW LEVEL SECURITY`, a `dfipam_app_all` policy for `dfipam_app`, and explicit least-privilege grants. Row level security stays as a second layer if the app role is ever misused (migration 000001 also revokes everything from `anon` and `authenticated`, roles left from the Supabase days, where they exist).
- **The audit row** is written in the same transaction as the change.
- **All SQL is parameterised;** sort and filter fields are allowlisted. Never one query per row.

## Roles

| Role | Used by | Can |
| --- | --- | --- |
| `postgres` | Migrations and seed (`MIGRATION_DATABASE_URL`) | Everything |
| `dfipam_app` | The API (`DATABASE_URL`) | Read and write the tables below as granted. No DDL |

`dfipam_app` grants:

| Tables | Grants | Why |
| --- | --- | --- |
| `users`, `reservations`, `ip_requests` | `SELECT`, `INSERT`, `UPDATE` | Accounts are deactivated and requests and reservations end in a status, never deleted |
| `audit_logs` | `SELECT`, `INSERT` | Append-only; a trigger also blocks updates, deletes and truncation |
| `network_link_providers`, `network_links`, `network_link_attachments` | `SELECT`, `INSERT`, `UPDATE` | Links are archived, providers switched off and attachments marked removed, never deleted |
| `network_link_relations`, `network_link_segments` | `SELECT`, `INSERT`, `DELETE` | Replaced as a whole when a link is saved; the audit record keeps the old ones |
| Every other table | `SELECT`, `INSERT`, `UPDATE`, `DELETE` | |

In Docker, `docker/postgres/init` creates `dfipam_app` with its password. On another PostgreSQL the first migration creates it without login; enable it once with `ALTER ROLE dfipam_app WITH LOGIN PASSWORD '<password>';`.

## Tables

| Table | Holds |
| --- | --- |
| `users` | Email, display name, role (a `roles` key, or none), active flag, auth subject |
| `roles` | Key, name, description, permissions, full access, system flag, order. Users refer to it by key |
| `segments` | Name, type (`dhcp`, `server`, `cloud`), floor, VLAN, network, gateway, DHCP pool start and end, default environment, description, alert-on-private-MACs and check-ports switches |
| `environments` | Name, colour, order, default severity, default monitor and critical for new servers |
| `departments` | Name and description. Devices and register entries hold the name as text |
| `devices` | MAC (unique), name, owner, department, type, platform, status (`pending`, `approved`, `rejected`), who decided and when, first and last seen, notes |
| `ip_addresses` | The register: IP (unique), segment, status (`assigned`, `reserved`, `blocked`), type, environment, device, assigned name, expected MAC, platform, services, owner, department, monitor, critical, check port, health (`up`, `down`, `unknown`) and failure count, notes |
| `reservations` | Start and end IP, segment, reason, owner, expiry, status (`active`, `converted`, `released`), reminder sent |
| `ip_requests` | Requester, segment, type, name, purpose, needed by, requested IP, status, decided by and when, reason, assigned IP |
| `sightings` | Evidence, one row per IP and MAC: segment, source (`ping`, `arp`, `netbios`, `dns`, `dhcp`, `vpn`), hostname, DHCP vendor class or device description, static, private MAC, gateway, network card maker, device maker, model and kind, TTL system hint, open ports, first and last seen |
| `alerts` | Type, severity, segment, register entry, device, IP, MAC, message, status (`open`, `acknowledged`, `closed`), dedupe key, times |
| `notifications` | Per person: kind, message, link, read, email status. Also the email outbox |
| `notification_preferences` | Per person and kind: email on or off |
| `settings` | Key and JSON value: scan and check intervals, failures before Down, reminder days, session timeout, email switch, FortiGate connection (token encrypted) |
| `sessions` | Microsoft sign-in sessions: user, SHA-256 hash of the cookie token, created, last seen, expires, revoked, address, browser |
| `audit_logs` | Append-only: actor, action, object type and ID, old and new values, time, source IP, correlation ID |
| `network_link_providers` | Name (unique, any case), account manager, contact number, email, support hotline and email, website, remarks, active |
| `network_links` | Link ID (`DF-LNK-0001`, from the `network_link_no` sequence), name, type (a key from `links.types`), provider, circuit ID (unique per provider among live links), source site, destination, purpose, environment, role, criticality, officer, status; bandwidth in Mbps, medium, WAN IP, WAN segment, more WAN addresses (`extra_wan_ips inet[]`, each with its subnet's prefix), public range, gateway, connected devices (`device_ids uuid[]`; devices are never deleted, the API checks new ones are approved) and other devices as text, interface, VLAN, VRF, routing; VPN tunnel, type, peers, remote subnets (`cidr[]`, IPv4, never local space), remote organisation and contact, last test, vault reference; no-separate-cost flag, billing account, monthly rental, currency, installation cost, contract and PO references, activation, contract start and end, notice days, cost centre, reminders sent; support contact, email, hotline, account manager, SLA %, resolution hours; remarks; archived at and by; created by |
| `network_link_relations` | Link, related link, kind (`secondary`, `backup`, `underlying`). One row per pair, never to itself |
| `network_link_segments` | A VPN link's local subnets: link and segment |
| `network_link_attachments` | Link, category (`contract`, `quotation`, `diagram`, `configuration`, `other`), file name, content type, size, SHA-256, stored name on the attachment volume, uploaded by and when, removed by and when |

## Enforced by the schema

`make test-db` checks these on a throwaway `dfipam_test` database.

- Segment networks never overlap. A DHCP pool sits inside its network, and only `dhcp` segments have one. (The API also keeps the gateway and pool off the network and broadcast addresses.)
- Register IPs and reservations sit inside their segment, and a segment can't be moved away from them.
- One IP appears once in the register; one MAC once in devices.
- A rejected request needs a reason; a decided request needs who decided and when.
- One open alert per cause (`dedupe_key`).
- The audit log is append-only.
- Numeric settings hold numbers and the email switch a boolean. Their ranges, such as a scan interval of 5 to 1,440 minutes, are checked by the API.

`sightings` is upserted on (IP, MAC), so it stays about as large as the number of devices. Rows unseen for 90 days, and alerts closed for a year, are deleted after each scan.

`seen_window()` is the one definition of "seen recently" (two scan intervals, at least 10 minutes), used by the alert rules, Discovery, segments and the dashboard. Go code uses it as `db.SeenWindow`, a subquery, so it is worked out once per query.

## Migrations

| # | Adds |
| --- | --- |
| 000001 | The schema, RLS, policies and grants |
| 000002 | Server check failure count |
| 000003 | Reservation reminder time |
| 000004 | Indexes for Discovery, alert and audit filters; settings limits |
| 000005 | Cloud segments; FortiGate evidence (DHCP leases, VPN sessions) |
| 000006 | Platform and services on devices and register entries |
| 000007 | Gateway evidence on sightings |
| 000008 | Per-segment alert on private-MAC devices |
| 000009 | Device details on sightings: card maker, maker, model, kind, TTL hint |
| 000010 | Departments |
| 000011 | Per-segment port checks, open ports, register lookup by MAC and device |
| 000012 | Roles that administrators set up, with six starting roles; `users.role` refers to them |
| 000013 | Sessions for Microsoft Entra ID sign-in |
| 000014 | `sightings.identified_at`; the `seen_window()` function; server checks every 5 minutes instead of once a day |
| 000022 | `sightings.entra_*`: a device's registered owner, name, join type and last sign-in from Microsoft Entra ID |
| 000024 | Index on open alerts by MAC, for the Devices list |
| 000023 | Network Links: the five `network_link*` tables, the `links.*` settings (types, sites, reminder days, USD rate), and the `links:*` permissions for the starting roles |
| 000027 | `network_links.extra_wan_ips` (more WAN addresses) and `device_ids` (several connected devices), replacing `device_id` |

The seed (`make seed`, safe to repeat) loads the five environments and default settings.
