# Roadmap

Each phase ends with something usable. The order puts the core inventory first, because everything else depends on it.

| Phase | Delivers | Exit criteria | Needs first |
| --- | --- | --- | --- |
| **0. Foundation** | Repo, three binaries, config, logging, migrations, CI, app shell, Supabase sign-in, RBAC, audit base | User signs in, roles are enforced, a change writes an audit row | Done: Supabase project, schema live |
| **1. Core IPAM** | Hierarchy, blocks, subnets, IP inventory, teams, DNS and DHCP records, overlap rules and exceptions, IPv4/IPv6 calculators, Explorer | Overlaps blocked per routing domain, including against exception subnets; edge prefixes tested | `btree_gist` confirmed on target DB |
| **2. Allocation workflow** | Requests, approvals, allocations, reservations, releases, expiry, notifications | Concurrency tests pass; no self-approval | Mail provider |
| **3. Imports and devices** | Production file import, device and interface inventory | Real files parse with no record dropped; reversal works | The two production files |
| **4. Discovery** | Policies, scheduler, agents, ICMP, ARP/NDP, reverse DNS, Nmap, safety controls. SNMP still to come | Scope and emergency stop tests pass in a lab | Redis approval; sensitive networks list |
| **5. Integrations and reconciliation** | FortiGate, vCenter, AWS, Azure, DNS/DHCP, reconciliation engine | Read-only confirmed per connector; rule tests pass | Service accounts; DNS/DHCP server types |
| **6. Reporting and dashboard** | Full dashboard, async reports, saved filters | Reports honour permissions at volume | |
| **7. Hardening** | Load tests, final sizing, security review, backups, recovery runbooks, company auth migration | Checklist in [security.md](security.md) complete; restore tested | Company auth details |

## Testing

| Level | Focus |
| --- | --- |
| Unit | Address calculators (`/31`, `/32`, `/127`, `/128`), file parsers, confidence rules |
| Integration | Real PostgreSQL: constraints, concurrent allocation, audit immutability |
| API | Permission matrix for every route, DTOs, error codes |
| Connector | Recorded fixtures; no live systems in CI |
| Safety | Discovery out of scope, exclusions, emergency stop, "discovery never writes allocation data" |
| End to end | Not built yet. API journeys run through the real router in `internal/server` |
