# Development Trace

Everything DF-IPAM has to deliver, and where it stands. Update this file when a feature changes status.

**Last updated:** 5 November 2026 (Phase 1 screens and remaining API)

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
| 1 | Core IPAM | ✅ Complete (exit check on Supabase still to run, see below) |
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

**Exit criteria:** overlaps blocked per routing domain, including against exception subnets; edge prefixes tested. Both pass in `make test-db` against PostgreSQL. Run it once more on Supabase after this change lands.

| Item | Status | Notes |
| --- | --- | --- |
| Organisations, routing domains | ✅ Done | List, create, edit and delete. Delete is refused while anything uses them |
| Sites, zones, VDOMs, VLANs, business units, services, applications, teams | ✅ Done | Network Management tabs: search, create, edit, delete. A VDOM needs its firewall as a device, which has no screen until Phase 3 |
| Network blocks | ✅ Done | Tab with free-space view: used share, free addresses, largest free CIDR |
| Subnet create, view, archive, filter, search | ✅ Done | Filters for site, VLAN, VDOM, service and block, sortable columns, archived toggle |
| Subnet edit | ✅ Done | Version-checked form with every mapping |
| Subnet delete | ✅ Done | Refused when records depend on it, with "archive instead" |
| Bulk subnet operations | ✅ Done | Edit fields, archive or delete up to 500 at once, all or nothing, one audit row per subnet |
| Subnet mapping to site, VLAN, block, service | ✅ Done | Contradictions are rejected |
| Subnet mapping to VDOM, zone, owners, business unit | ✅ Done | In the form. Owners come from `GET /users/lookup`, which needs only `inventory:view` |
| Overlap prevention | ✅ Done | Enforced by the database |
| Overlap exceptions | ✅ Done | Tab for approvers (`requests:approve`), applied from the subnet form. Expired ones can't be applied, and removing one while the overlap remains is refused |
| IPv4 and IPv6 calculations, `/31`, `/32`, `/127`, `/128` | ✅ Done | |
| Utilisation breakdown | ✅ Done | Subnet detail view |
| Free ranges | ✅ Done | Subnet detail view |
| Provider-reserved ranges | ✅ Done | Created with every subnet in an AWS VPC or Azure VNet routing domain: the first four addresses and the last |
| Split, summarise, locate tools | ✅ Done | Tools dialog on Subnet Management |
| Address grid | ✅ Done | Colour-coded, 256 addresses per page, jump to any address. Large IPv6 subnets open on their recorded addresses |
| IP records: create, edit, search, history, evidence | ✅ Done | Bulk edit of owner, service, application and allocation state (up to 500) |
| IP Address Explorer screen | ✅ Done | Table and grid views, filters, detail view with history, evidence and DNS records |
| DNS and DHCP records | ✅ Done | Read-only `GET /dns-records` and `GET /dhcp-scopes/:id/reservations`. DHCP scopes and reserved ranges are managed in the subnet detail view. Filling them from DNS and DHCP servers is Phase 5 |
| Saved filters, column visibility | ✅ Done | Saved filters are per user on the server. Column choice is per browser |
| Global search | ✅ Done | Header search across subnets, IP records, blocks, sites, VLANs, VDOMs, devices and DNS records. An address or CIDR matches by containment |

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
