# Development Trace

Everything DF-IPAM has to deliver, and where it stands. Update this file when a feature changes status.

**Last updated:** 5 November 2026

| Status | Meaning |
| --- | --- |
| ✅ Done | Built and tested, backend and screen |
| 🟦 API done | Backend built and tested, screen still to build |
| 🟨 Partial | Some of it works: the note says what is missing |
| ⬜ To do | Not started |
| ⛔ Blocked | Needs something from outside the team |
| ➖ Deferred | Decided against for now |

## Progress

| Phase | Scope | Status |
| --- | --- | --- |
| 0 | Foundation | ✅ Complete |
| 1 | Core IPAM | 🟨 Backend mostly done, screens remain |
| 2 | Allocation workflow | ⬜ Schema only |
| 3 | Imports and devices | ⛔ Device API done, imports need the production files |
| 4 | Discovery | ⬜ Schema only |
| 5 | Integrations and reconciliation | 🟨 Findings engine done, integrations not started |
| 6 | Reporting and dashboard | 🟨 Basic dashboard only |
| 7 | Hardening | ⬜ Not started |

**Foundation exit check passed:** a real user signed in, roles were enforced, and creating an organisation through the screens wrote an audit row on Supabase.

## Phase 0: Foundation

| Item | Status | Notes |
| --- | --- | --- |
| Project documentation | ✅ Done | `docs/` |
| Repository, Makefile, gitignores, `CLAUDE.md` | ✅ Done | |
| Env files in `api/` and `web/` | ✅ Done | |
| Go API, worker, agent, migrate and seed programs | ✅ Done | All start and connect. The agent stays idle until Phase 4. The worker runs reconciliation only |
| Config, structured logs, correlation IDs | ✅ Done | |
| Migrations and seed | ✅ Done | 8 migrations live on Supabase |
| Single-file SQL setup | ✅ Done | `api/sql/supabase-setup.sql` |
| Supabase sign-in, token verification | ✅ Done | Tested with real ES256 signatures. MFA not set up |
| Roles and permissions | ✅ Done | Every route is guarded, enforced by a test |
| Audit logging | ✅ Done | Same transaction as the change, append-only |
| Data API lockdown | ✅ Done | Row level security on every table |
| CORS, rate limits, security headers | ✅ Done | |
| App shell, sidebar, header, account menu, centred modals | ✅ Done | Light theme only |
| Pagination | ✅ Done | Same metadata on every list, page state in the URL, dropdowns load all options |
| CI pipeline, PR template, Dependabot | ✅ Done | Linted, and every step passes locally. First GitHub run waits for a remote |
| Dependency and secret scanning | ✅ Done | No known reachable vulnerabilities. No secrets in git history |
| OpenAPI contract | ✅ Done | Every route is documented, enforced by a test |

## Phase 1: Core IPAM

| Item | Status | Notes |
| --- | --- | --- |
| Organisations, routing domains | ✅ Done | List and create, used with real data. No edit or delete yet |
| Sites, zones, VDOMs, VLANs, business units, services, applications, teams | 🟦 API done | Used by the New subnet form where relevant, but no management screens |
| Network blocks | 🟦 API done | Includes free-space analysis |
| Subnet create, view, archive, filter, search | ✅ Done | |
| Subnet edit | 🟦 API done | Version-checked. No edit form yet |
| Subnet delete | 🟦 API done | Blocked when records depend on it |
| Bulk subnet operations | ⬜ To do | |
| Subnet mapping to site, VLAN, block, service | ✅ Done | Contradictions are rejected |
| Subnet mapping to VDOM, zone, owners, business unit | 🟦 API done | Not in the form yet |
| Overlap prevention | ✅ Done | Enforced by the database |
| Overlap exceptions | 🟦 API done | Enforced by the database, no screen |
| IPv4 and IPv6 calculations, `/31`, `/32`, `/127`, `/128` | ✅ Done | |
| Utilisation breakdown | ✅ Done | Subnet detail view |
| Free ranges | ✅ Done | Subnet detail view |
| Provider-reserved ranges | 🟦 API done | Counted in utilisation. Not created automatically yet |
| Split, summarise, locate tools | 🟦 API done | |
| Address grid data | 🟦 API done | Range-based, safe for IPv6 |
| IP records: create, edit, search, history, evidence | 🟦 API done | |
| IP Address Explorer screen | ⬜ To do | Grid, table and detail view |
| DNS and DHCP records | 🟨 Partial | DHCP scopes have an API. `dns_records` is read by findings but has no API |
| Saved filters, column visibility | ⬜ To do | Table exists |
| Global search | 🟨 Partial | Subnets only |

## Phase 2: Allocation workflow

| Item | Status | Notes |
| --- | --- | --- |
| Direct allocate, reserve, release by an authorised user | 🟦 API done | Release needs a reason. Keeps allocation history |
| Allocation requests | ⬜ To do | Tables exist |
| Multi-level approvals, separation of duties | 🟨 Partial | Self-approval is blocked by the database. No workflow yet |
| Automatic next-free selection | ⬜ To do | Approach tested during design |
| Concurrent allocation and reservation test | ⬜ To do | Design test covered allocations only |
| Reservation expiry and reminders | ⬜ To do | |
| Notifications | ⬜ To do | Needs a mail provider |
| Alerts page | ⬜ To do | Findings count shows in the header |
| Invite users | 🟨 Partial | Built, but no real invitation has been sent |
| User role assignment | ✅ Done | Screen built. Not yet exercised with a second real user |

## Phase 3: Imports and devices

| Item | Status | Notes |
| --- | --- | --- |
| Device and interface API | 🟦 API done | |
| Device and interface screens | ⬜ To do | |
| Duplicate device matching | ⬜ To do | |
| Production file parser | ⛔ Blocked | Needs `Prod_Subnets.txt` and `Prod_VDOMs_Subnets.txt` |
| Import preview, approval, reversal, enrichment queue | ⬜ To do | Tables exist |

## Phase 4: Discovery

| Item | Status | Notes |
| --- | --- | --- |
| Policies, schedules, jobs, exclusions | ⬜ To do | Tables exist. Job snapshots are frozen by the database |
| Agent registration, heartbeat, credential rotation | ⬜ To do | |
| ICMP, ARP/NDP, DNS, SNMP, Nmap collectors | ⬜ To do | |
| Safety controls and emergency stop | ⬜ To do | Setting is seeded, nothing uses it yet |
| Discovery Centre screen | ⬜ To do | |
| Redis job queue | ⛔ Blocked | Needs Redis approval |
| Sensitive network rules | ⛔ Blocked | Needs the list of banking and payment networks |

## Phase 5: Integrations and reconciliation

| Item | Status | Notes |
| --- | --- | --- |
| Reconciliation engine, all 11 classifications | ✅ Done | Never changes inventory. Discovery-based rules have no evidence to work on yet |
| Findings screen and review | ✅ Done | |
| Hourly worker run | ✅ Done | Ran on Supabase and wrote a system audit row. Hourly repeats not yet observed |
| Configurable source priority and confidence | ⬜ To do | Rules are in code today |
| Change requests from findings | ⬜ To do | |
| FortiGate, vCenter, AWS, Azure connectors | ⬜ To do | Tables exist |
| DNS and DHCP connectors | ⛔ Blocked | Needs the server types |
| Encrypted credential storage | ⬜ To do | |
| Integrations screen | ⬜ To do | |

## Phase 6: Reporting and dashboard

| Item | Status | Notes |
| --- | --- | --- |
| Dashboard totals, most utilised subnets, allocation state, recent changes | ✅ Done | |
| Trend charts | ⬜ To do | Needs daily utilisation snapshots |
| Excel, CSV and PDF reports | ⬜ To do | |
| Audit log screen | ✅ Done | Filters still to add |

## Phase 7: Hardening

| Item | Status | Notes |
| --- | --- | --- |
| Load tests and final sizing | ⬜ To do | |
| Security review | ⬜ To do | Checklist in `docs/security.md` |
| Nginx, systemd units, backups, runbooks | ⬜ To do | |
| Roles and Permissions, System Administration screens | ⬜ To do | |
| Company auth migration | ⛔ Blocked | Needs company auth details |

## Waiting on others

| Need | From | Blocks |
| --- | --- | --- |
| Rest of the main brief after section 26, if any | Project owner | Final scope |
| Two production export files | Project owner | Imports |
| DNS and DHCP server types | Network team | Connectors |
| List of sensitive networks | Network and security | Active discovery |
| Redis approval | Infrastructure | Job queue |
| Cloud data approval | Compliance | Production data |
| RPO, RTO, retention, inventory size | Business | Backups and sizing |

## Security to-do

- Turn off public sign-ups in Supabase.
- Change the database password and `service_role` key, because both were shared in chat.
