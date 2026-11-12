# Changelog

What changed, day by day, on branch `lite`. Newest first. The specification is [new.md](new.md); what is and isn't built is in [development_phase.md](development_phase.md).

## 2026-11-12

**Devices and servers**
- The Devices table shows current presence without the first-discovered timestamp; first-seen details remain in the device and Discovery dialogs.
- Label the device IP as the last-seen address and explain that DHCP can change it, separately from a deliberate IP register entry.
- The empty Servers view points to reviewed server imports and registering reviewed Discovery findings.
- Network setup shows full-width segment cards in the compact Discovery style; four cards fit across on wide screens. Cards focus on kind and subnet details, with scan/open actions, without pool utilization or zero-count activity summaries.
- Add/edit segment shows a live subnet preview: mask, broadcast, usable range, address counts and DHCP pool overlay.
- Dell is selectable as a device platform; Dell and HP hostname labels are recognized.

**Discovery**
- Counts above the findings that filter: Needs action, Open alerts, Undecided devices, Known, New in the last hour.
- A details dialog per address, with the device, its network card and its state in DF-IPAM; the main decision (Approve, Register) as a button on each row.
- The device is told apart from its network card: maker, model and kind from UPnP and mDNS, a likely system from the ping reply's TTL, the maker from the device's name. Migration 000009.
- Optional per-segment checks of about 25 common ports, showing the services behind them. Migration 000011.
- An approved device registered at another address shows "Registered elsewhere" and counts as known.
- Approving, registering or editing re-checks alerts at once, so warnings clear without waiting for a scan.

**Steady scanning**
- Port checks share 16 connections per scan; mDNS, UPnP and ports at most every 6 hours per device, or on Scan now.
- No rescan when the backend restarts; at least 5 minutes between scans; sightings unseen for 90 days are removed.
- A segment's gateway no longer raises "unregistered static" or "unknown server IP" alerts (the router on Wi-Fi did; every firewall interface would have).

**Network setup**
- Settings split into separate pages: Network setup (segments, environments, departments), Users and roles, Audit log, System (settings, FortiGate).
- Departments, picked from a list on device and register forms. Migration 000010.
- Segments screen: counts that filter, search, sorting, groups by type, usage, activity and discovery icons, an Addresses button. Segment pages link back to where they were opened from.
- The `dhcp` segment type is now called **DHCP network** (a floor or Wi-Fi), not "Floor"; the optional floor number stays.
- 71 server networks loaded from the two FortiGates' `show system interface` output (local database only, not in git).

**Imports**
- Every import is reviewed in a dry run before anything is saved.
- Text files (`.txt`, `.tsv`) as well as CSV and Excel; segments from FortiGate output or CIDR lines; servers on the Servers tab.
- Imports really take 5 MB (the body limit had been 1 MB).

**People and sign-in**
- Roles set up by administrators from twelve permissions, with six starting roles (Administrator, Network engineer, IT officer, Security auditor, Requester, Viewer). Nobody can lock out user management. Migration 000012.
- Microsoft Entra ID sign-in for `@dialogfinance.lk`, alongside Supabase: PKCE and nonce, pinned tenant, HttpOnly session cookie, CSRF header, audited. Migration 000013.
- The sign-in page leads with **Sign in with Microsoft**; email and password fold away below it, and open by themselves while Microsoft sign-in isn't set up.

**Interface**
- Devices, Servers and the IP register show what the scanner knows (maker, model, likely system, network card, open services, open alerts, where a device is registered, a MAC that doesn't match); clicking a device, server or address opens a details dialog with everything about it and its actions (`?device=`, `?entry=`, `?finding=` in the address).
- Dashboard rebuilt: headline figures; every network by type with the most active ones; latest alerts; DHCP networks; servers, the register and devices.
- Sidebar: collapse at the top, live counts beside Devices, Alerts and IP Requests, version and build at the bottom.
- Header: who is signed in (opens their details) and Sign out.
- The bell shows open alerts; every new alert is an in-app notification, and High and Critical ones also go by email.
- Settings edits list exactly what changes before saving; Save stays disabled until something changed.

**Deployment and docs**
- Apache can replace the web container: `deploy/apache/df-ipam.conf`, `make apache-up`, `make apache-web`.
- Documentation rewritten to describe Lite as built; enterprise-only documents removed.

**Commits**
- `56b0a21` richer discovery, departments, server import and steady scanning
- `4de9a6c` docs describe Lite as built; enterprise-only documents removed
- `f26fcec` roles, Microsoft sign-in, separated admin pages, reviewed imports, Apache
- `dfb6808` dashboard, details dialogs, DHCP network naming, Microsoft-first sign-in

**Still to do**
- Fill in `ENTRA_TENANT_ID`, `ENTRA_CLIENT_ID` and `ENTRA_CLIENT_SECRET` from the Entra ID app registration, then sign in with Microsoft and give that account the Administrator role.
- `make docker-up` to apply migrations 000012 and 000013 to the local database.
- Remove Supabase sign-in when Microsoft sign-in is confirmed on the real tenant.

## 2026-11-11

- DF-IPAM Lite planned and founded: one building, IPv4 only, 13 tables, three containers.
- Inventory: segments, the IP register with Available candidates, devices with approval, CSV and Excel import and export.
- The built-in scanner: ping, reverse DNS, NetBIOS, ARP table, mDNS and SSDP; server checks by ping or port.
- Alerts with deduplication and summaries, the notification bell and email outbox.
- One-step IP requests, reservations with reminders, nine reports in CSV, Excel and PDF, the audit screen.
- Discovery screen with segment overview, address maps and free ranges; gateways detected; cloud segments; optional read-only FortiGate.
- Optional alerts for private-MAC devices per segment; Discovery shows what is connected now by default.
