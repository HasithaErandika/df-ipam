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
| 1 | Core IPAM | ✅ Complete, except live checks on Supabase |
| 2 | Allocation workflow | ✅ Complete. Email waits for a mail provider |
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
| Go API, worker, agent, migrate and seed programs | ✅ Done | All start and connect. The agent stays idle until Phase 4. The worker runs reconciliation hourly, and allocation expiry and email every minute |
| Config, structured logs, correlation IDs | ✅ Done | |
| Migrations and seed | ✅ Done | 8 migrations live on Supabase. 9 and 10 (allocation workflow, device classes) are written and tested, not yet applied there |
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
| Split, summarise, locate tools | ✅ Done | Tools dialog on Subnet Management |
| IP Address Explorer | ✅ Done | Table and colour-coded grid, filters, record detail with history and evidence, create and edit |
| Device view per address | ✅ Done | Device, interface, MAC flags, DNS, DHCP and reporting sources, from inventory and approved read-only sources only |
| Device types and OS family, with icons | ✅ Done | Laptops, desktops, phones, access points, servers, VMs, firewalls and more. Filled by people, imports and, later, discovery |
| DNS and DHCP records | ✅ Done | Read-only. DNS records list through the API and appear in the device view |
| Saved filters, column visibility | ✅ Done | Saved filters on Subnets and the Explorer. Column choice on Subnets is kept in the browser |
| Global search | ✅ Done | Subnets, addresses, hostnames, MACs, devices and VLANs. An address finds its subnet in every routing domain |
| Live check on Supabase | ⬜ To do | Apply migrations 9 and 10, then run `make test-db` |

## Phase 2: Allocation workflow

| Item | Status | Notes |
| --- | --- | --- |
| Allocation, reservation and release requests | ✅ Done | Draft, submit, cancel. Release and conversion requests point at their allocation or reservation |
| Multi-level approvals, separation of duties | ✅ Done | Workflows choose role-based steps by request type, environment and sensitivity. No self-approval, one step per person, steps in order. Enforced in the service and the database |
| Approval workflow administration | ✅ Done | Workflows tab on IP Allocation Requests. Submitted requests keep their steps |
| Fulfilment: allocate, reserve, release | ✅ Done | Only approved requests. Repeating a call returns the same result |
| Automatic next-free selection | ✅ Done | Range-based, safe for IPv6. Skips held, observed, reserved-range, DHCP, gateway and more-specific-subnet addresses |
| Concurrent allocation and reservation tests | ✅ Done | Real database, separate connections: 40 allocations 20 at a time, exhaustion, allocation against reservation for one address, racing approvers, repeated fulfilment |
| Direct allocate, reserve, release by an authorised user | ✅ Done | From the Explorer's edit dialog. Release needs a reason |
| Reservation expiry, extension and reminders | ✅ Done | Reminder a week before; expiry returns the address to the candidate pool. Extend needs a reason, at most a year ahead |
| Stale request expiry | ✅ Done | Approved but unfulfilled after 30 days, or a reservation whose end date passed first |
| Notifications | 🟨 Partial | In-app alerts, rules, per-person preferences and deduplication work. Email is built (SMTP, STARTTLS) but waits for a mail provider |
| Alerts page | ✅ Done | Inbox, settings, event rules, and what needs attention. Unread count in the header |
| Requests, Reservations screens | ✅ Done | |
| Invite users | 🟨 Partial | Built, but no real invitation has been sent |
| User role assignment | ✅ Done | Screen built. Not yet exercised with a second real user |

**Phase 2 exit check:** concurrency tests and the no-self-approval tests pass on PostgreSQL 16. They still need one run on Supabase with `make test-db`.

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
| Mail provider (SMTP host or Microsoft Graph) and a sender address | Infrastructure | Email notifications |
| Cloud data approval | Compliance | Production data |
| RPO, RTO, retention, inventory size | Business | Backups and sizing |

## Security to-do

- Turn off public sign-ups in Supabase.
- Change the database password and `service_role` key, because both were shared in chat.
