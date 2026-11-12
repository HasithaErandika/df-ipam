# Development Status

What is built, what isn't, and what waits on others. Update this when a feature changes status. The specification is [new.md](new.md).

## Built

| Area | Status |
| --- | --- |
| Foundation | 16 tables with RLS, policies and grants; roles and permissions; route guard; audit in the same transaction; Docker (`db`, `migrate`, `backend`, `web`) |
| Segments | Floor, server and cloud segments; DHCP pools; gateways (set or detected); private-MAC alerts and port checks per segment |
| IP register | Create, edit, release; Available candidate; range reservations; CSV and Excel import and export, including servers |
| Devices | Pending, approve, reject; register on approval; departments; import and export |
| Discovery | Ping, ARP table, DNS, NetBIOS, mDNS, SSDP/UPnP, optional port checks; device details apart from the network card; registered-elsewhere detection; findings with counts, details and filters; segment pages with an address map |
| Steady scanning | Rate and concurrency limits, slow probes at most every 6 hours, no rescan on restart, 90-day evidence retention |
| Server checks | Ping or check port, Down after N failures, environment severities |
| Alerts | Nine rules, dedupe, quiet reopen, summaries, re-checked after every inventory change |
| Requests and reservations | One-step approval; holds with reminders, extend, convert, release |
| Notifications | Bell, email outbox (off by default), per-type preferences, hourly limit |
| Reports and audit | Nine reports in CSV, Excel and PDF; audit screen with filters and export |
| Roles and users | Roles set up by administrators from twelve permissions, six starting roles, never locked out; users given roles |
| Administration | Network setup (segments with import from text, environments, departments), Users and roles, Audit log, System (settings, FortiGate) as separate pages; every edit confirmed with its list of changes |
| Imports | Devices, register and servers from CSV, text or Excel, and segments from FortiGate output; every import reviewed in a dry run first |
| Apache | Optional host Apache in place of the web container: site file, compose override, `make apache-up` and `make apache-web`; syntax and a live proxy test passed |
| FortiGate | Optional read-only ARP, DHCP leases and VPN sessions |
| Microsoft sign-in | Entra ID authorization code flow with PKCE and a nonce, pinned tenant, `@dialogfinance.lk` only, HttpOnly session cookie with idle and 10-hour limits, CSRF header, audited; alongside Supabase |

## Not built

| Item | Waiting on |
| --- | --- |
| ARP sweep on each floor VLAN from a trunk port | A trunk port from the network team |
| SNMP read of the core switches (MAC tables, LLDP and CDP neighbours) | Read-only SNMP access |
| Removing Supabase sign-in | Your go-ahead; Microsoft sign-in tested on the real tenant first |

## Waiting on others

| Item | From |
| --- | --- |
| The segment list, DHCP pools and server environments | Network team |
| The real server range (`172.168.x.x` or `172.16`–`172.31`) | Network team |
| Agreement to run port checks on company segments | Security team |
| Alert email recipients, and who approves requests | IT management |
| An existing device list to import | Asset owners |
| The Entra ID app registration (tenant ID, client ID, secret, redirect URIs) | IT / identity team |
| A rehearsed deployment, backup restore and key rotation | Operations, see [runbooks.md](runbooks.md) |

## Security to-do

- Turn off public sign-ups on the Supabase project.
- Run the [release checklist](security.md#release-checklist) before real data is loaded.
