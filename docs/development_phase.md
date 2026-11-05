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
| 1 | Core IPAM | ✅ Complete |
| 2 | Allocation workflow | ✅ Complete. Email waits for a mail provider |
| 3 | Imports and devices | 🟨 Built. Parser needs checking against the real production files |
| 4 | Discovery | 🟨 Built. Needs a lab run, the sensitive-networks list and SNMP credentials |
| 5 | Integrations and reconciliation | 🟨 Built. Connectors need trying against real systems. DNS and DHCP servers blocked |
| 6 | Reporting and dashboard | 🟨 Built. Reports tested through the API. Screens not checked by hand |
| 7 | Hardening | 🟨 Screens, Docker and runbooks built. Load test, security review and restore not yet run. Company auth blocked |

**Testing status, 5 November 2026:** `make lint test` and all 47 database and API tests pass, on a fresh `dfipam_test` database built from all 14 migrations. Migrations 13 and 14 aren't on the Supabase test database yet: run `make migrate-supabase`. No screen has been checked by hand since the frontend restructure, and no agent has sent real packets.

**Foundation exit check passed:** a real user signed in, roles were enforced, and creating an organisation through the screens wrote an audit row on Supabase.

## Phase 0: Foundation

| Item | Status | Notes |
| --- | --- | --- |
| Project documentation | ✅ Done | `docs/` |
| Repository, Makefile, gitignores, `CLAUDE.md` | ✅ Done | |
| Env files in `api/` and `web/` | ✅ Done | |
| Go API, worker, agent, migrate and seed programs | ✅ Done | All start and connect. The agent stays idle until Phase 4. The worker runs reconciliation hourly, and allocation expiry and email every minute |
| Config, structured logs, correlation IDs | ✅ Done | |
| Migrations and seed | ✅ Done | 14 migrations on local PostgreSQL. Supabase has the first 12 |
| Docker | ✅ Done | `web`, `backend` and `db` through `docker-compose.yml`, with `dev` and `prod` overrides. `make docker-up` starts all of it. Production TLS config checked, not yet run on a server. The real client address needs checking from a second machine |
| Sample network | ✅ Done | `make seed-demo` loads 190 subnets and full workflows through the real API. Development only |
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
| Organisations, routing domains | ✅ Done | Create, edit and delete. Delete only when nothing uses them |
| Sites, zones, VDOMs, VLANs, business units, services, applications, teams | ✅ Done | One management screen for each, under Network Management |
| Network blocks | ✅ Done | Management screen with free-space view |
| Subnet create, view, edit, archive, delete, filter, search | ✅ Done | Edit is version-checked. Delete is blocked when records depend on the subnet |
| Bulk subnet operations | ✅ Done | Bulk create (all or nothing, with a dry run per row) and bulk archive |
| Subnet mapping to site, VLAN, block, service, VDOM, zone, owners, business unit | ✅ Done | Contradictions are rejected |
| Overlap prevention | ✅ Done | Enforced by the database |
| Overlap exceptions | ✅ Done | Management screen for approvers; chosen when creating a subnet |
| IPv4 and IPv6 calculations, `/31`, `/32`, `/127`, `/128` | ✅ Done | |
| Utilisation breakdown, free ranges | ✅ Done | Subnet detail view |
| Provider-reserved ranges | ✅ Done | AWS and Azure reservations are recorded when a subnet is created in a VPC or VNet routing domain |
| Split, summarise, locate tools | ✅ Done | Tools dialog on Subnets |
| IP Addresses | ✅ Done | Table and colour-coded grid, filters, record detail with history and evidence, create and edit |
| Device view per address | ✅ Done | Device, interface, MAC flags, DNS, DHCP and reporting sources, from inventory and approved read-only sources only |
| Device types and OS family, with icons | ✅ Done | Laptops, desktops, phones, access points, servers, VMs, firewalls and more. Filled by people, imports and, later, discovery |
| DNS and DHCP records | ✅ Done | Read-only. DNS records list through the API and appear in the device view |
| Saved filters, column visibility | ✅ Done | Saved filters on Subnets and the Explorer. Column choice on Subnets is kept in the browser |
| Global search | ✅ Done | Subnets, addresses, hostnames, MACs, devices and VLANs. An address finds its subnet in every routing domain |
| Live check on Supabase | ✅ Done | Migrations applied, and database tests passed on 5 November 2026 |

## Phase 2: Allocation workflow

| Item | Status | Notes |
| --- | --- | --- |
| Allocation, reservation and release requests | ✅ Done | Draft, submit, cancel. Release and conversion requests point at their allocation or reservation |
| Multi-level approvals, separation of duties | ✅ Done | Workflows choose role-based steps by request type, environment and sensitivity. No self-approval, one step per person, steps in order. Enforced in the service and the database |
| Approval workflow administration | ✅ Done | Workflows tab on Requests. Submitted requests keep their steps |
| Fulfilment: allocate, reserve, release | ✅ Done | Only approved requests. Repeating a call returns the same result |
| Automatic next-free selection | ✅ Done | Range-based, safe for IPv6. Skips held, observed, reserved-range, DHCP, gateway and more-specific-subnet addresses |
| Concurrent allocation and reservation tests | ✅ Done | Real database, separate connections: 40 allocations 20 at a time, exhaustion, allocation against reservation for one address, racing approvers, repeated fulfilment |
| Direct allocate, reserve, release by an authorised user | ✅ Done | From the Explorer's edit dialog. Release needs a reason |
| Reservation expiry, extension and reminders | ✅ Done | Reminder a week before; expiry returns the address to the candidate pool. Extend needs a reason, at most a year ahead |
| Stale request expiry | ✅ Done | Approved but unfulfilled after 30 days, or a reservation whose end date passed first |
| Notifications | 🟨 Partial | In-app alerts, rules, per-person preferences and deduplication work. Email is built (SMTP, STARTTLS) but waits for a mail provider |
| Alerts page | ✅ Done | Inbox, settings, event rules, and what needs attention. Unread count in the header |
| Requests, Reservations screens | ✅ Done | |
| Invite users | 🟨 Partial | Sends the Supabase invitation, but no real one has been tried, and the app has no screen to set or reset a password. Create accounts in the Supabase dashboard for now |
| User role assignment | ✅ Done | Screen built. Not yet exercised with a second real user |

**Phase 2 exit check passed:** concurrency tests and the no-self-approval tests pass on PostgreSQL 16 and on Supabase.

## Phase 3: Imports and devices

| Item | Status | Notes |
| --- | --- | --- |
| Device and interface inventory screens | 🟦 Built, not tested | Device types and OS family with icons |
| Duplicate device matching and merge | 🟦 Built, not tested | By serial, interface MAC, management IP or hostname |
| Production file parser | ⛔ Blocked | Tolerant parser built (CIDR, address and mask, VDOM-prefixed lines). Needs `Prod_Subnets.txt` and `Prod_VDOMs_Subnets.txt` to confirm no line is dropped |
| Import preview, second-person approval, reversal, enrichment queue | 🟦 Built, API tested | Every line kept with a status. Upload, correction, approval and reversal tested on PostgreSQL. Screen not checked by hand. Uploads over 1 MB are refused by the global body limit, so the 5 MB limit can't be reached yet |

## Phase 4: Discovery

| Item | Status | Notes |
| --- | --- | --- |
| Policies with second-person approval, schedules, exclusions, blackouts | 🟦 Built, API tested | Saving a policy clears its approval. A schedule runs at most every 15 minutes |
| Frozen job scope | 🟦 Built, API tested | Allowlist, exclusions win, blackouts, IPv4 up to /20, IPv6 only at recorded addresses, 65,536 addresses per job. Multicast, loopback and link-local refused |
| Agent registration, heartbeat, rotation, revocation | 🟦 Built, API tested | One-time tokens; credentials stored as hashes. Registration, claiming and results tested through the real API |
| ICMP, ARP/NDP, reverse DNS, Nmap host discovery | 🟦 Built, not run | Limits are unit tested, but no agent has sent real packets. Every packet is rate limited, and the agent holds its own ceilings |
| SNMP collector | ⬜ To do | Needs SNMPv3 credential storage (Phase 5 encrypted credentials) |
| Safety controls and emergency stop | 🟦 Built, API tested | Traffic ceiling across agents, limits rechecked between targets, results rechecked against scope. An agent that loses the server stops itself. Discovery never changes allocation or ownership |
| Discovery screen, Explorer scan button | 🟦 Built, not tested | |
| Redis job queue | ⛔ Blocked | Jobs use PostgreSQL with SKIP LOCKED meanwhile |
| Sensitive network rules | ⛔ Blocked | Mechanism built (passive only unless an approved sensitive policy); needs the list of banking and payment networks |
| Lab test of scope and emergency stop | ⬜ To do | Phase 4 exit criterion |

## Phase 5: Integrations and reconciliation

| Item | Status | Notes |
| --- | --- | --- |
| Reconciliation engine, all 11 classifications | ✅ Done | Never changes inventory. Stale VMware evidence raises no findings |
| Findings screen and review | ✅ Done | |
| Hourly worker run | ✅ Done | Ran on Supabase and wrote a system audit row. Hourly repeats not yet observed |
| Configurable source priority and confidence | ✅ Done | Rules and confidence tab. Scores are capped and explained. Starting weights need review |
| Change requests from findings | ✅ Done | Second person required, enforced in the service and database. Allocation state can't travel this way |
| FortiGate connector | 🟨 Built | GET only. Tested against fixtures built from published API shapes, not a real device |
| vCenter connector | 🟨 Built | Tested against VMware's simulator, not a real vCenter |
| AWS connector | 🟨 Built | Assumes a read-only role. Tested with a fake EC2 client, not a real account. Cloud subnets are kept on evidence only |
| Azure connector | 🟨 Built | Reader role. Tested with a fake client, not a real subscription. Public IPs, load balancers and application gateways aren't collected yet |
| DNS and DHCP connectors | ⛔ Blocked | Needs the server types. FortiGate DHCP works through the FortiGate connector |
| Encrypted credential storage | ✅ Done | AES-256-GCM envelope, tied to each connection, with key versions and `make docker-rotate-secrets` |
| SSRF protection | ✅ Done | Allowlist, address checks after DNS, connects to the checked address, no redirects |
| Integrations screen | 🟦 Built, not viewed | Connections, tests, syncs, history, routing map |
| Read-only confirmed per connector | ⛔ Blocked | Exit criterion. Needs the real service accounts |

## Phase 6: Reporting and dashboard

| Item | Status | Notes |
| --- | --- | --- |
| Dashboard totals, most utilised subnets, allocation state, recent changes | ✅ Done | |
| Trend and distribution charts | 🟦 Built, not viewed | Daily snapshots from the worker, so trends fill in over time. Hand-built SVG rather than Recharts |
| Excel, CSV and PDF reports | ✅ Done | 13 reports, in the background. PDF stops at 5,000 rows and says so. Formulas are neutralised |
| Reports screen and summary | 🟦 Built, not viewed | |
| Scoped reports | 🟨 Partial | Business Viewers get only their business services. Exit criterion "at volume" not yet measured |
| Audit log screen and filters | 🟦 Built, not viewed | Actor, action, object, result and dates |

## Phase 7: Hardening

| Item | Status | Notes |
| --- | --- | --- |
| Roles and Permissions screen | 🟦 Built, not viewed | Edit permissions and details, create and delete roles. Keeps someone able to manage users |
| System Administration screen | 🟦 Built, not viewed | Health, queues, retention |
| Retention pruning | 🟨 Built | `make docker-retention`, daily timer. Audit logs are never pruned automatically. Not yet run |
| Docker, Nginx, timers, backup script | 🟨 Built | Docker checked locally. Nginx, timers and backup not yet run on a server |
| Setup guide, system guide and runbooks | 🟨 Written | `docs/setup.md`, `docs/runbooks.md`, and a system guide PDF kept outside git. Not rehearsed |
| Load tests and final sizing | ⬜ To do | Tool built (`make load-test`), not run. Needs a test system and the real inventory size |
| Security review | 🟨 Partial | Data leak and flooding review done on 5 November 2026: see `docs/security.md`. `make vuln` and the full checklist not run |
| Restore test | ⬜ To do | Exit criterion |
| Company auth migration | ⛔ Blocked | Needs company auth details |
| Code size and query rules | ✅ Done | Files under 200 lines, function complexity 15 or less in production code, no query per row on read paths |
| Feedback, confirmations and error handling | 🟦 Built, not viewed | Toasts, confirmation of edits and deletes, friendly errors, sign-out when a session ends |
| Screens load on demand, tabs are paths | 🟦 Built, not viewed | Old `?tab=` links redirect |
| Account menu and details dialog | 🟦 Built, not viewed | Top bar shows a user icon; the menu has Details and Sign out |

## Waiting on others

| Need | From | Blocks |
| --- | --- | --- |
| Rest of the main brief after section 26, if any | Project owner | Final scope |
| Two production export files | Project owner | Imports |
| DNS and DHCP server types | Network team | Connectors |
| List of sensitive networks | Network and security | Active discovery |
| Redis approval | Infrastructure | Job queue |
| Mail provider (SMTP host or Microsoft Graph) and a sender address | Infrastructure | Email notifications |
| Service accounts, confirmed read-only | Platform owners | Trying the connectors |
| RPO, RTO, retention, inventory size | Business | Backups and sizing |

## Security to-do

- Turn off public sign-ups in Supabase.
- Change the database password and `service_role` key, because both were shared in chat.
- Add a screen to set and reset passwords.
- Fix the global 1 MB body limit, which also caps the 5 MB import and 8 MB agent routes.
