# Status

What is built, what isn't, and what waits on others. Update this when a feature changes status. What each feature does is in [product/](product/overview.md).

## Built

| Area | Status |
| --- | --- |
| Foundation | 16 tables with RLS, policies and grants; roles and permissions; route guard; audit in the same transaction; Docker (`db`, `migrate`, `backend`, `web`) |
| Segments | Floor, server and cloud segments; DHCP pools; gateways (set or detected); host-address and pool checks; warnings for segments too large to scan or without a pool; private-MAC alerts and port checks per segment |
| IP register | Create, edit, release; Available candidate; range reservations; CSV and Excel import and export, including servers |
| Devices | Pending, approve, reject; register on approval; departments; import and export |
| Discovery | Ping, NetBIOS, TCP presence check for hosts that block ping, ARP table (ARP-only hosts too), DNS, mDNS, SSDP/UPnP, optional port checks; ARP MAC preferred over NetBIOS; NetBIOS computer name by name type; device details apart from the network card; device icons from the evidence in order of reliability; red and orange for what needs attention; registered-elsewhere detection; findings with counts, shareable details and filters; segment pages with an address map; devices pending as soon as their segment is scanned; static addresses in use for 30 days after last seen; a failed segment scan keeps its alerts open |
| Steady scanning | One ICMP socket per scan; 100 pings and 100 TCP connections a second; full identification at most every 6 hours (15 minutes on Scan now), remembered across restarts; no rescan on restart; send failures reported; 90-day evidence and one-year closed-alert retention |
| Server checks | Every hour by default; two pings, then the check port; Down after N failures; response time recorded, Slow server alert over a limit; batched updates; environment severities |
| Alerts | Nine rules, dedupe (device alerts per MAC), DHCP-aware conflicts, quiet reopen, summaries, batched writes, re-checked after every inventory change |
| Requests and reservations | One-step approval; holds with reminders, extend, convert, release |
| Notifications | Bell, email outbox (off by default), per-type preferences, hourly limit |
| Reports and audit | Nine reports in CSV, Excel and PDF; audit screen with filters and export |
| Roles and users | Roles set up by administrators from twelve permissions, six starting roles, never locked out; users given roles |
| Administration | Network setup (segments with import from text, environments, departments), Users and roles, Audit log, System (settings, FortiGate) as separate pages; every edit confirmed with its list of changes |
| Imports | Devices, register and servers from CSV, text or Excel, and segments from FortiGate output; every import reviewed in a dry run first |
| Apache | Optional host Apache in place of the web container: site file, compose override, `make apache-up` and `make apache-web`; syntax and a live proxy test passed |
| FortiGate | Optional read-only ARP, DHCP leases (counted as present only when ARP confirms them) and VPN sessions |
| Microsoft sign-in | Entra ID authorization code flow with PKCE and a nonce, pinned tenant, `@dialogfinance.lk` only, HttpOnly session cookie with idle and 10-hour limits, CSRF header, audited. The only way in, locally and on servers; the API won't start in production without it or HTTPS. Supabase and the development sign-in are removed |
| First administrator | `make admin EMAIL=…`; pending accounts that a person's first Microsoft sign-in takes over |

## Not built

| Item | Why it matters | Waiting on |
| --- | --- | --- |
| ARP sweep on each floor VLAN from a trunk port | Every MAC, even with firewalls on | A trunk port from the network team |
| SNMP read of the core switches (ARP, MAC tables, LLDP and CDP neighbours) | MACs across routed networks, and the switch port of each device | Read-only SNMP v2c or v3 on the switches |
| Reading the floor DHCP server directly | Exact DHCP or static, without relying on the pool range | Read access to the DHCP server. The FortiGate leases cover it if it serves DHCP |
| **Add person** in Users and roles | Give a role before someone's first sign-in from the screen, not only with `make admin` | Nothing: next to build |

## Waiting on others

| Item | From |
| --- | --- |
| The segment list, DHCP pools and server environments | Network team |
| The real server range (`172.168.x.x` or `172.16`–`172.31`) | Network team |
| Agreement to run port checks on company segments, and the TCP presence check (22, 445, 3389) | Security team |
| Alert email recipients, and who approves requests | IT management |
| An existing device list to import | Asset owners |
| A rehearsed deployment, backup restore and key rotation | Operations, see [runbooks.md](operations/runbooks.md) |

## Security to-do

- Delete the old Supabase project, now unused, and its keys.
- Run the [release checklist](engineering/security.md#release-checklist) before real data is loaded.
