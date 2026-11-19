# Changelog

What changed, day by day, on branch `lite`. Newest first. The specification is in [product/](product/overview.md); what is and isn't built is in [status.md](status.md).

## 2026-11-19

**Discovery**
- NetBIOS name queries now get answers from Windows PCs on other networks. The scanner asked for the wildcard name padded with spaces and flagged as a broadcast; Windows ignores that. It now sends the query exactly as `nbtstat -A` does (`*` padded with zero bytes, unicast). On routed networks such as other floors and Wi-Fi, Windows PCs now show their computer name and network card MAC without reverse DNS or the FortiGate.
- The UPnP search sent to each device is now a proper unicast search: `HOST` names the device and there is no `MX`. Some printers, TVs and routers ignored the multicast-style search and so gave no maker or model.
- **Stop** next to Scan now ends a running scan (`POST /api/v1/discovery/stop`, permission `discovery:scan`, audited as `scan_stop`, after a confirmation). Segments already scanned keep their results and the alert rules still run; the segment in progress saves nothing and isn't marked as failed. The status line then says "Last scan stopped".

**IP register and reservations**
- An address held by an active reservation can't be registered by hand (409 `IP_RESERVED`) or by import (the row is skipped and reported). Before, it could, and the address ended up both assigned and still reserved. Convert the reservation instead. Approving a request already checked this.
- The message when a new reservation overlaps an address in use now gives the real rule: 30 days outside the DHCP pool, 24 hours inside it. It said 24 hours.

## 2026-11-17

**Sign-in: production only**
- The development sign-in is gone: no `AUTH_BYPASS` or `AUTH_BYPASS_EMAIL`, no `POST /api/v1/auth/bypass`, no button or warning banner. Microsoft Entra ID is the only way in, locally too.
- With `APP_ENV=production` the API refuses to start without `ENTRA_TENANT_ID`, `ENTRA_CLIENT_ID`, `ENTRA_CLIENT_SECRET` and an `https://` `APP_BASE_URL`.
- The sign-in page says plainly when sign-in isn't available. A new test covers a `make admin` pending account taken over by the first Microsoft sign-in.
- Remove `AUTH_BYPASS` and `AUTH_BYPASS_EMAIL` from your `api/.env`; they are ignored now.
- Plain `http://` is still allowed in production for `localhost`, `127.0.0.1` and `::1`, where it never crosses the network; any other host needs `https://`. The first rule stopped a local container started with `APP_ENV=production`.

**Security and dependencies**
- Go 1.26.11 (`go.mod`, `api/Dockerfile`), which fixes GO-2026-6859, a crash in `net/http`. `govulncheck` now reports nothing that the code calls.
- Web dependencies from Dependabot's #12 (React Query, React Hook Form and its resolvers, Lucide, Simple Icons, Tailwind, Vite and its React plugin), applied on `lite`. `@types/node` stays on 24 to match Node 24 in `web/Dockerfile`, not 26.

- The Entra ID setup now says to grant admin consent; without it Microsoft shows "Need admin approval".

**Web server (nginx)**
- Device, register and segment imports up to 5 MB get through: nginx refused anything over 2 MB with 413 before it reached the API.
- Files under `/assets/` keep the security headers (`nosniff`, CSP, and HSTS over HTTPS); a cache header added there had dropped them.

**Discovery and alerts**
- A static address seen in the last 30 days counts as in use (24 hours inside the DHCP pool, as before). A device switched off for a weekend or a leave is no longer offered as an Available candidate, and an approval or reservation can't take its address.
- On routed segments, a sighting without a MAC borrows the MAC last seen at its address only outside the DHCP pool. Inside it the address may belong to another device by now, which was credited to the old one and could raise a false "Rejected device is active".
- When a segment's scan fails, its alerts stay open until a scan succeeds, instead of closing as if resolved (migration 000015: `segments.scan_failed_at`). Segments too large to scan aren't counted as failed.
- Server checks that can't ping now skip servers without a check port, and log it, instead of counting failures until every one of them is Down.
- Monitored servers are checked every hour by default, not every 5 minutes (migration 000016 changes the setting only where it was still 5 minutes). With 3 failed checks before Down, an outage now shows after about 3 hours.

**Automatic scanning switches and slow servers**
- Settings › System has three switches: scan device segments automatically, scan server segments automatically, and check monitored servers automatically. They stop only scheduled work; Scan now still scans any segment. While a kind of segment isn't scanned automatically its alerts stay open, and while checks are off servers keep their last state.
- Each server check records how fast the server answered: the faster of two pings, or the check port's connect time. It shows next to the status on Servers and in the server's details.
- Discovery shows "Automatic scanning is off. Use Scan now." instead of a next scan time when both scan switches are off, and says when the next scan covers only device or only server segments.
- New **Slow server** alert (Medium) when a monitored server answers slower than the limit in Settings (default 200 ms). Migration 000017 adds `ip_addresses.response_ms` and the alert type.

**Microsoft sign-in fix**
- Sign-in failed with "didn't complete" for every account: Microsoft publishes its signing keys without `alg`, and the JWT library uses no key without one. Microsoft's RSA keys are now pinned to RS256 when fetched (never inferred), and the test's fake Microsoft publishes its keys the same way, so the test catches it.

**Text export**
- Every export (Devices, IP register, Servers, Audit log and Reports) can download tab-separated text (.txt) as well as CSV and Excel: one line per row, Windows line endings for Notepad, columns when pasted into a spreadsheet, and it imports back as is. A tab or line break inside a value becomes a space, and values that look like formulas are neutralised as in CSV.

**Commits**
- `94a8c01` remove the development sign-in; Entra ID only, and production needs it with HTTPS

## 2026-11-13 · v0.2.0

**Finding and identifying devices**
- Hosts that block ping (Windows by default, many servers) are found through NetBIOS and a TCP check of ports 445, 3389 and 22, and from the ARP table when nothing else answers. Only a completed connection counts.
- The ARP MAC wins over the NetBIOS one, which on hosts with several network cards is often another card's. This stops false "IP conflict" and "Server MAC changed" alerts.
- A sighting whose NetBIOS reply was missed joins the MAC last seen at that address, instead of being listed again without one.
- The NetBIOS computer name is chosen by name type, never a signed-in user's name. mDNS is asked even when DNS gives a name, so Apple models are found.
- Devices found in a segment become pending, and raise their alerts, as soon as that segment is scanned, not after the whole sweep.

**Steadier scanning**
- One ICMP socket per scan, replies matched by sequence number and sender; failed sends are reported on the segment.
- At most 100 TCP connections a second across a scan. Scan now identifies a host in full at most every 15 minutes, and full identification is remembered across restarts (migration 000014).

**Alerts and monitoring**
- On DHCP pool addresses, an IP conflict needs both MACs within one scan; DHCP handing an address on is no longer a conflict.
- Device alerts follow the MAC: a device that DHCP moves keeps its one alert.
- Server checks run every 5 minutes by default instead of once a day, with two pings before a failure counts. "Server back up" is sent only when it really is.
- A FortiGate DHCP lease counts as seen only when the firewall's ARP table confirms it.
- A DHCP network with no pool no longer marks every address static. Closed alerts are removed after a year.

**Efficiency**
- Alerts and server health are written in one statement each, not one per row. Discovery looks up home entries and alert counts for the shown page only. The segment list counts in one pass. "Seen recently" has one definition, `seen_window()`.

**Validation**
- Gateways and DHCP pools must use host addresses, not the network or broadcast address (form and API). A gateway inside the pool is allowed, with a note in the form, since many DHCP servers lease the whole range and exclude their own address.
- The segment form warns when a segment is too large to scan (over 4,096 addresses), and segment cards say so first.
- Dates are checked against the person's own calendar day.
- The Settings screen shows the seeded default for a setting without a saved value, instead of its minimum. The seed stays the one source of starting settings and environments, with a test that keeps them inside each setting's limits.

**Screens**
- Device icons come from the evidence in order of reliability: what a person recorded, the router mark for a gateway, what the device says it is, its model, DHCP vendor class or name, its open ports, a specific ping TTL (128 Windows, 255 network gear), then its maker. A TTL of 64 no longer shows Android, and the network card's maker is never used. Discovery, segment pages, Devices and the register all use the same rules.
- Discovery colours what needs attention: red for open alerts and rejected devices, orange for unknown devices and addresses that should be in the register. An approved device on a DHCP pool address stays plain.
- Discovery: a details link opens whatever page or filter is showing, the list pauses while a dialog is open, "Not answering · last seen …" replaces "Unavailable since …", and the default sort is labelled "Last seen, newest".
- The Platform field in the approve, device and register forms is a menu with each platform's mark instead of a native list, which some browsers closed or couldn't scroll inside a dialog.
- The dashboard's network tiles and "All segments" open Discovery › Segments, filtered to that type.
- The header shows the signed-in person's name from their work email (hasitha.wickramasinghe@… shows as Hasitha Wickramasinghe), with their initials in the avatar.
- The side panel shows the version and build, without the build date. Version 0.2.0.

**Sign-in: Supabase removed**
- Supabase is gone: its sign-in, bearer tokens, admin client, SQL bundle (`make sql`, `api/sql/`), test targets, settings and the `@supabase/supabase-js` package. PostgreSQL in Docker is the only database, and the API's session cookie is the only way a request is signed in.
- Microsoft Entra ID is the way in. A person's first Microsoft sign-in takes over a pending account made for their email, keeping its role.
- Development sign-in (`AUTH_BYPASS`, `AUTH_BYPASS_EMAIL`) for while there is no Entra ID app registration: one button, one named person, made an Administrator the first time. The API refuses it with `APP_ENV=production`, which the server compose files now fix, and every screen shows a warning while it is on.
- `make admin EMAIL=…` makes someone an Administrator, before or after their first sign-in. Audited.

**Documentation**
- Reorganised into `product/` (the specification), `engineering/`, `operations/`, `status.md` and this changelog. `new.md` is split across `product/`, and `plan.md` is retired now that Supabase is removed.

**Fixed during the day**
- Saving a sweep failed with a syntax error after the scan changes: a column in the insert was named `full`, a reserved word in PostgreSQL. Renamed, with a database test that saves a sweep end to end. It never reached a release.

**Local environment**
- The local database was cleared and rebuilt with all 14 migrations and the seed (five environments, default settings). `hasitha.wickramasinghe@dialogfinance.lk` was made an Administrator with `make admin`.
- `api/.env` no longer holds Supabase settings, and has the development sign-in on for that account.

**Commits**
- `22b8b7c` find and identify devices accurately without flooding the network
- `34d25c6` tighter segment validation, clearer Discovery and segment screens; v0.2.0
- `2edd836` remove Supabase; Entra ID sign-in, with a development sign-in until it is registered
- `091a425` restructure the documentation and describe sign-in without Supabase
- `e285411` save sightings again, and update devices as each segment finishes
- `31f4b42` correct device icons, attention colours, platform menu and names in the header

**Still to do**
- Fill in `ENTRA_TENANT_ID`, `ENTRA_CLIENT_ID` and `ENTRA_CLIENT_SECRET` from the Entra ID app registration, sign in with Microsoft (it takes over the pending Administrator account), then set `AUTH_BYPASS=false`.
- Delete the old Supabase project and its keys.
- Decide whether to keep the five starting roles besides Administrator (they come from migration 000012).
- Agree with the security team on the TCP presence check (22, 445, 3389) and on port checks for company segments.
- Build **Add person** in Users and roles, so roles can be given before someone's first sign-in from the screen.
- Push the commits.

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
