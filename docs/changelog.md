# Changelog

What changed, day by day, on branch `lite`. Newest first. The specification is in [product/](product/overview.md); what is and isn't built is in [status.md](status.md).

## 2026-11-26

**Email: only what needs someone** ([alerts.md](product/alerts.md#email))
- Critical alerts and request decisions go **at once**; High alerts and new IP requests are gathered into **one digest** every 30 minutes; a **daily summary** at 08:00 lists only what needs attention (open alerts, approvals waiting, networks 90% full, failing FortiGate, Intune or Entra ID reads and scans, link contracts ending within 30 days, your reservations expiring), by permission, and nothing on a quiet day. Medium and Low alerts, servers back up and discovery findings stay in the app.
- **Team mailbox**: alert and request email can go to one address (a distribution list) instead of to every handler.
- **Professional HTML email** with a plain-text copy, in the app's colours, every line linking into the app; **Send a test email** in Settings.
- System › Settings › Email shows what is sent when; Email preferences list the four kinds a person's role can get.
- Migration 000031: `notifications.email_mode` and `severity`, and team-mailbox notices with no user.

Fixes from a security audit (security-audit skill) and code-quality reviews of the API and web app.

**Access and sign-in**
- **Converting a reservation needs `inventory:edit` or `requests:approve`.** A Requester holds and releases addresses, but gets one assigned only through an IP request; reserving and then converting skipped the approver. The Reservations screen shows "Assign an address" only to people who may.
- **Reports need the screen's own permission.** The Audit log report now needs `audit:view`, and the network, device and request reports need `inventory:view`, besides `reports:export`. Before, any report exporter (Viewer, Requester, IT officer, Network engineer) could download the audit trail.
- **A user manager can't take away what they don't hold.** Editing a role checks removed permissions as well as added ones, so it can't strip access from people who hold more (`403 BEYOND_OWN_ACCESS`).
- **IDs in paths are lower-cased**, so `PUT /users/<own id in capitals>` no longer slips past the self-change rule.
- **On HTTPS only the `__Host-` session cookie signs in.** A plain `dfipam_session` cookie, which another host under the same domain could plant, is ignored. The sign-in attempt cookie is `__Host-dfipam_login` on HTTPS too, and sign-ins in progress are counted per IPv6 /64, like the rate limit.
- **Link change history hides contract and quotation file names** from people without `links:commercial_view`, as the audit log does.
- **Apache with `HOST_NETWORK=true`** adds `docker-compose.apache-hostnet.yml`: the backend trusts Apache on loopback, so each person keeps their own address for rate limits and the audit log.
- `TRUSTED_PROXIES` with an entry that isn't an address or network now stops the API at start, instead of being ignored silently. `APP_ENV` is read case-insensitively everywhere.

**Imports and data rules**
- **Spreadsheet imports keep at most 256 columns per row.** One cell in column XFD padded every row to 16,384 cells, so a 15 KB file could hold gigabytes in memory.
- **A /32 segment can hold its one address** (migration 000029: the checks use `<<=`).
- **Editing a register entry keeps the gateway rule:** the gateway stays type Network.
- **Approving a device** registers its address with the device's stored name and platform, and the segment's default environment.
- **Pasted segment lists follow the form's rules:** prefixes from /16 to /32 only, no gateway on the network or broadcast address, and long names cut by character.
- A reservation made for someone else takes their name, not the creator's.
- **Email retries back off** 1, 2, 4 then 8 minutes (migration 000030), and email isn't sent to deactivated people. Leading dots in emails are no longer doubled.

**Web app**
- The Add server and register entry dialogs no longer clear what was typed when lists refresh in the background, nor reset Type, Monitor and Critical by hand.
- FortiGate, Intune and System settings forms keep unsaved edits (including a pasted token) while the status refreshes.
- Server errors on fields without their own message (platform, services, type) show as a form message instead of nothing.
- A reservation's expiry badge counts calendar days: "Today" on its last day.
- Approve, reject, edit-reservation, convert and edit-user dialogs no longer show the previous record's error.
- "free" is gone from the Available addresses card and the address map legend.
- **Shared links to a device or register entry open its details** whatever page, tab or filter is showing: the record is fetched by id (`?id=` on `/devices` and `/ip-addresses`). Discovery's finding dialog uses the same query-string handling as the others, and closing a dialog goes back instead of adding a second copy of the list to the history.
- **Every time shows in Sri Lanka time**, with one formatter: the network links pages and the rest of the app no longer disagree.
- Brand colours, the dialog backdrop and the menu, dialog and pill shadows are tokens in `index.css`; the sign-in page uses the shared logo.

**Background work**
- After a restart the scan schedule keeps asking the database when segments were last swept, instead of taking a failed read as "never" and sweeping the whole network straight away.
- The network link import reads providers' circuit IDs once, not per row, and a database error is reported instead of being taken as "no match", which used to add a duplicate link.

**Dependencies** (Dependabot #18–#22)
- Kept: pgx 5.13.0 and `x/net` 0.63.0, the web dependencies (React Query, React Router, Lucide, Simple Icons, Vitest, Hook Form resolvers), Nginx 1.31, and `pnpm/action-setup` v6.
- Reverted: **Postgres stays on 16**. Postgres 19 can't open the existing `df-ipam-lite-db-data` volume, so the database wouldn't start; a major upgrade needs a dump and restore. The **Go image stays on 1.26.11**, matching `go.mod` and what CI tests, and **Node stays on 24**. Dependabot now leaves these majors alone.
- pnpm 11.12.0 → 11.29.0 (`web/package.json`). 11.12.0's self-installer fails in `pnpm/action-setup` v6, which broke the Web and Security CI jobs; fixed from 11.13.
- The complexity lint no longer fails on a fresh CI cache: Go's "downloading" messages were counted as findings.

## 2026-11-23

Fixes from a full review of the code (tech lead, security and dependency reviews), and the BCP site's links.

**Access**
- **Nobody hands out more than they hold.** Someone who manages users but isn't an Administrator can't add a permission they lack to any role, change their own role's permissions, give a role that holds more than theirs, or change a person who holds more. Refused with `403 BEYOND_OWN_ACCESS` or `409 SELF_CHANGE`. Changes to who can manage users are serialised, so two at once can't leave nobody able to.
- **Managing link costs brings seeing them.** A role saved with `links:commercial_manage` also gets `links:commercial_view` (migration 000028 adds it to existing roles). Without view, the link form was sent the costs blank and saving wiped them. The API now writes commercial fields only for people with both, and the form shows the Commercial tab only to them.
- **Sign-in** reads the person's sign-in name (UPN) before the email claim, which people can edit, and refuses Entra guest accounts. Each address can hold at most 5 sign-ins in progress, so one client can't push out everyone else's.
- **Exports and report downloads are audited** (action `export`, with the format and filters).

**Data rules**
- **Segment edits follow the register's rules:** a DHCP pool can't be moved over register entries, nor the gateway onto an entry that isn't network equipment. A segment a network link uses can't be deleted, and the message says so.
- **Converting a reservation** checks the address against the register's rules, audits the holds left either side, and keeps a reminder already sent.
- A save pointing at something that no longer exists answers `422`, not `409`.
- Devices can't be deleted by the app (they never were), so the ids in a link's connected devices stay valid.
- The **FortiGate token is removed when the address changes** without a new token, so it is never sent to a different host.

**Reliability**
- **Requests have a 55-second limit**, and cancelled or timed-out requests roll back. A slow import can no longer commit after the person was told it failed.
- **Excel uploads are capped** at 64 MB unpacked and 20,000 rows, so a small file can't fill memory. The backend's `/tmp` is 128 MB and its memory 1 GB (`BACKEND_MEM_LIMIT`).
- **Discovery sweeps only private networks** (10.x, 172.16–31.x, 192.168.x, 100.64–127.x). A floor or server segment with a public range says "Public range: not swept".
- **The scan schedule counts from the last finished sweep** (`segments.last_swept_at`, migration 000028), not from the newest ARP sighting, which the FortiGate reader writes too.
- The scanner and the FortiGate reader update sightings in the same order, so they can't deadlock.
- A change in the register waits at most 3 seconds for an alert evaluation already running, instead of holding the request until it ends.

**Network links**
- **Create the WAN subnet's segment from the link form.** When the WAN IP's network isn't a segment, people with `network:manage` get **Create segment 10.10.40.0/24**: a small dialog with the network filled in and the type set to **Cloud** (never swept). The new segment becomes the link's WAN subnet.
- The import says when a device or other name belongs to more than one record, instead of picking one, and a row can relate to a link added earlier in the same file.
- An export matching more than 20,000 links is refused with a message, not cut short.
- The overview's approximate rupee total names the currencies it leaves out.
- **Seeded:** the Head Office and BCP - Nugegoda links from the two site diagrams (14 links, 13 segments), through the import with audit records.

**Web**
- Several requests failing at once on an ended session give one message and one sign-out; the idle timer signs out once.
- Downloads start reliably before their file address is released.
- Marking a notification read says so. Unused `patch` helpers are removed.

**Engineering**
- **CI runs the database tests** in a new `API (database)` job with PostgreSQL, and fails if any skip.
- `make lint` checks the 200-line file and complexity-15 function rules (`make lint-size`), and so does CI.
- Base images are pinned by digest and watched by Dependabot. nginx 1.30, Alpine 3.24, `golang.org/x/crypto` 0.59.0.
- On the host network, the API no longer trusts `127.0.0.1` for the client address.

## 2026-11-20 · v0.4.0

- The app version is 0.4.0 (side panel and API description).

**Network links from more than one site, and addresses as the diagrams write them**
- **Source location** in the link dialog has a **+** for people with `network:manage`: a new location (such as "BCP - Nugegoda") is added to the list in Network setup > Network links and chosen for the link. Others are told who adds locations.
- **WAN IP and gateway** accept an address with its prefix or mask (`10.10.40.253 /24`, `122.255.4.226 255.255.255.248`) and keep the address. Leaving the WAN IP picks the matching segment as the WAN subnet, or says which network isn't a segment yet. **Public IP / range** accepts a dotted mask. The API and the import accept the same forms.
- **Import** sits beside Add network link on the Overview and All Network Links (for `links:import_export`). In the file, an empty **WAN Subnet** uses the WAN IP's prefix when that network is a segment, and the template's help sheet shows the accepted address forms.
- **More WAN addresses on one link**, such as head office DAP_ILL's two public `/32`s: **Add WAN address** under the main WAN IP, each a subnet then an address. Stored with their prefix in `network_links.extra_wan_ips` (migration 000027); searched, found from a register entry or segment, shown on the link page, in reports, and in the import and export column **More WAN IPs**.
- **Several connected devices, found by the scan.** **Connected devices** takes as many approved devices as the link has; `device_ids` replaces `device_id` (migration 000027 moves existing ones). The dialog asks `GET /network-links/seen-devices` which devices the scan saw in the last 30 days on the WAN addresses and gateway: on a new link the approved ones are added for you to check before saving, on an edit they are offered. Unapproved ones are named. The link page shows what the scan saw, apart from what is recorded. Import column **Connected Devices** (several, `;`); old **Connected Device** files still import.
- **Link page:** a **WAN subnets and addresses** card lists every WAN subnet (the segment, or the network when it isn't one) with the WAN addresses in it, the main one marked, each linked to its IP register entry when there is one. A **Connected devices** card shows each device's MAC, type, approval status and where and when the scan last saw it, devices kept as text, and other devices the scan saw on the link's addresses that aren't recorded on it.
- **A link can have several segments**, for any link type: the Technical tab's **Segments** field (a VPN's local subnets moved there from the VPN tab). Stored in the existing `network_link_segments`, so no migration. The import and export column is now **Segments**; files with **Local Subnets** still import. Excel's extra sheet is **Subnets**.
- **WAN subnet comes before WAN IP.** Once a subnet is chosen, the WAN IP is offered from the register addresses in it, and an address outside it is pointed out.
- **Segments can be `/31` or `/32`**, for WAN hand-offs and single addresses. A `/32` has no gateway or DHCP pool; text import keeps such lines instead of skipping them.

**FortiGate connection on or off, and deleting the token**
- **System > FortiGate** has a **Connection on** switch. Off, DF-IPAM doesn't contact the firewall at all: scheduled reads, Read now, Test connection, Firewall > Live and Firewall > Import stop, and the settings and token are kept. It is on unless switched off, so existing setups carry on. Saved through the usual confirmation and audit record.
- **Delete** beside the API token erases the stored token (`DELETE /integrations/fortigate/token`, `system:manage`) after a confirmation, and switches the scheduled read off. The address and other settings stay. Recorded in the audit log.

**Firewall Import** (was Review and import)
- Renamed to **Import** at `/firewall/import`; the old address redirects.
- One **Fetch from firewall** reads networks, devices, servers and network links together, from a start panel that explains the three steps.
- Tabs show how many items wait for a decision. Each list opens on **To review**, hiding what is already up to date, registered or approved, with **All** beside it, and has **Choose all**.
- One save bar, stuck to the bottom of the screen, shows how many are chosen, what is still missing, **Clear** and **Save N…**. The saved results can be dismissed.
- The Devices list fits the page without scrolling sideways.

**Review fixes**
- **Audit log:** people without `links:commercial_view` no longer see Network Links' commercial fields, or contract and quotation file names, in the audit log or its export, and can't find them by searching.
- **Register rules on the server:** registering by hand, approving a device with its address, and importing all refuse the network and broadcast addresses, DHCP pool addresses, the gateway (unless its type is Network) and held addresses. Imports say why each row was skipped. Approving a device no longer gets past a reservation.
- **Reservations** can't cover the network or broadcast address, the gateway or the DHCP pool.
- **Imports:** Critical with Monitor left blank turns monitoring on instead of rejecting the row.
- **Departments** must come from the list, matched without case and stored in the list's spelling, including in imports.
- **Sri Lanka time:** the database session uses Asia/Colombo, and date checks use Sri Lanka's calendar day. Reservation expiries end at midnight in Sri Lanka; migration 000026 moves existing ones from midnight UTC. Report and audit date ranges follow Sri Lankan days.
- **Expiring holds** on the dashboard and monitor count only holds still to expire in the next 7 days, matching the Expiring tab; overdue ones are under Expired.
- **Server checks:** an entry whose monitoring was turned off during a check stays Unknown.
- **Sign-in:** when many sign-ins are in progress, only the oldest is dropped, not others at random.
- **Approving a device:** leaving out owner or department keeps them; sending them blank clears them. The FortiGate device review no longer sends blank values.

**Default departments**
- New installs and existing databases start with IT, Finance, Marketing, Cyber Security, Management and HR (migration 000025). Added once, so a department someone deletes or renames does not come back.

**Monitor view fits one screen**
- The status in the top band is a single labelled strip: "Needs attention" followed by each problem with its count ("2 Servers down"), without the pulsing dot and loose pills. When nothing is wrong it reads "All systems normal".
- The monitor shows only summary charts. It no longer fetches the alert list or the list of servers down, and the Needs attention, Servers down, Waiting for someone and Busiest networks panels are gone (their counts stay in the tiles and charts).
- On a wide screen it fills exactly one screen and does not scroll. The page behind it is locked, so Chrome no longer scrolls past the monitor into empty space.

**Tables fit the page**
- The Devices, Servers and IP register tables no longer scroll sideways. They use fixed column widths, long text is cut short or wraps inside its cell, and less important columns still hide on narrow screens. Other tables are unchanged.

**Network Links** (new module, [network-links.md](product/network-links.md))
- A new menu section records the company's internet lines, broadband, VPN tunnels, MPLS, leased lines, 4G/5G, cloud connections and third-party links: **Overview**, **All Network Links**, **Add Network Link**, **Service Providers**, **Contract Renewals** and **Import / Export**. Records are administrative; nothing is polled or configured.
- Each link has basic, technical, VPN (for VPN types only), commercial and support details, related links (secondary, backup, or the underlying line a VPN runs over; never itself, one row per pair), remarks, attachments and a field-by-field change history. Link IDs are `DF-LNK-0001` onwards. Links are archived and restored, never deleted.
- **Commercial information** is hidden on the server from people without `links:commercial_view`: in lists, details, the overview, exports, history and contract or quotation files. Without `links:commercial_manage`, saving a link leaves its commercial fields alone. Totals stay per currency unless an approved LKR per USD rate is set; terminated links and links with no separate cost are left out.
- **Contract reminders** go to the responsible officer and everyone with `links:commercial_manage` at 90, 60, 30, 7 and 0 days (Sri Lanka time), once each per contract end. A later expiry date is recorded as a renewal and starts the reminders again.
- **Attachments** of up to 15 MB are kept on a new private Docker volume (`df-ipam-lite-link-files`), checked against their content, and downloaded only through the API. The encrypted backup now includes them.
- **Excel import** with a template, a dry run, duplicate detection by Link ID or provider and circuit ID, and updates only when confirmed; **export** of the filtered list with VPN subnet and related link sheets.
- Segment pages and register entry details list the links that use them. **Network setup > Network links** holds the link types, sites, reminder days and USD rate.
- A searchable dropdown (`SearchSelect`, `MultiSearchSelect`) joins the UI kit, and lists can offer their own rows-per-page choices.
- The link dialog keeps one size whatever tab is open, and attachments get a drop zone with file icons and checks before saving.
- **Add network link** and **Edit** open as centred dialogs over the list and the link's page, not as pages of their own, and Add isn't a menu item: the Overview and list have the button. The dialog has numbered tabs with Back and Next, red marks on tabs with problems, and takes **attachments**, uploaded when the link is saved. Support details are no longer entered per link: the service provider gives the support, and the link's page shows its contacts.
- **Reports** adds Network links, Network link costs and Network link contract renewals. A report can now also need permissions beyond `reports:export`: it is left out of the list for people without them, and refused if asked for directly. Forward-looking reports (renewals) take dates ahead and default to the next 90 days.
- Eight new permissions (`links:*`); the starting roles get the ones that fit them. Migration 000023 adds the five tables and the settings. Nginx and the Apache site allow 15 MB attachment uploads.
- Fixed: the backup script recorded its last success in a `system_settings` table that Lite doesn't have, so every backup ended in an error after writing the file.

**Firewall** ([firewall.md](product/firewall.md))
- **Firewall is its own menu section**: Live, and Review and import. The connection stays in System > FortiGate. Review and import now also brings in **networks** (LAN, DMZ and VLAN interfaces with their DHCP pools, and SSL VPN and dial-up IPsec address pools as VPN segments), **devices** (address table, DHCP leases and the firewall's device identification, approved with a suggested name, type and platform) and **servers** (single-host address objects, virtual IPs and DHCP reservations as register entries), each matched to what DF-IPAM has and saved only after confirmation through the normal routes. Live shows **VPN users**, and the Monitor counts them. Addresses moved to `/firewall/live`, `/firewall/links/review`, `/firewall/links/apply` and the new `/firewall/inventory`; old links redirect.
- **Firewall** (first version, under Network Links): a **Live** view of the FortiGate's interfaces (state and download and upload rates against the link's bandwidth), IPsec tunnels, SD-WAN health and CPU, memory and sessions, read every 10 seconds at most and never stored, each matched to its link; a live card on matching link pages. **Review and add** fetches the WAN interfaces and IPsec tunnels as proposals (new, up to date, or with differences), people fill in the business details and tick what to add or update, and confirming reads the firewall again before saving. Tunnels are linked to the line they run over. GET only.

**Monitor view and dashboard**
- **Monitor view** (`/monitor`, button on the Dashboard) for a TV: the DF-IPAM logo in a slim one-line top band naming each problem; compact text with an icon on every tile and panel; Network links and Waiting for someone panels (devices, requests, holds, pending links, contracts); networks by type (online, registered, usable) and the busiest networks as on the dashboard; status, clock, tiles, alerts by severity, server health, live WAN traffic, open issues, servers down and DHCP pools, full screen. The idle sign-out pauses on this page only. The **Dashboard** gains a status line and a Network links card.

**Code review**
- **Review fixes:** an index for open alerts by MAC (migration 000024), which the Devices list counts on every row; pages no longer ask every 10 seconds for a firewall that isn't connected; confirming many firewall updates reads the names lookup once instead of once per link; a firewall interface no longer matches a VPN link that shares its interface name; a scanner test that could never fail now can; unused scanner code removed. Static analysis, dependency audits (Go and web) and the complexity check were run: no vulnerabilities, and the remaining complexity is in older functions listed in status.md.

**Discovery**
- **Users from Entra ID.** Each Intune read also lists Entra ID's devices with their registered owner, for company devices that aren't enrolled in Intune. They match by computer name (Entra keeps no MAC); a re-joined PC's most recently signed-in copy counts. Discovery shows the owner when Intune has no user (Intune, then Entra ID, then VPN), the finding details add a Microsoft Entra ID section, search finds the owner, and the Device users report names "Entra ID" as the source. System > Microsoft Intune shows Entra ID's device count, matches and error on their own, so a missing permission doesn't stop the Intune read. Needs the Graph application permissions `Device.Read.All` and `User.Read.All`. Migration 000022 adds the `entra_*` columns to `sightings`.
- **Register and Approve start filled in from discovery.** From a Discovery row or its details, the register form now gets the user (Intune, then Entra ID, then VPN) as owner, the likely platform (filled in, not only offered), and a note with maker and model, serial, OS and domain; the approval gets the user as owner, the likely platform, a device type from the evidence (PC, laptop, phone, printer, camera, network device, server), and the register type for its IP. Both say the values came from discovery and should be checked. An approved device's own record still replaces these guesses in the register form, and anything the person typed stays.

## 2026-11-19

**Reports, audit and import**
- New report **Device users and management**: devices seen in the last 30 days with their user (Intune or VPN), domain or workgroup, Windows version and Intune record (serial, maker and model, OS, compliance, encryption, last check-in).
- New report **VPN sessions**: the latest session on each tunnel address in the date range, with user, VPN kind and origin. Earlier sessions on the same address are not kept: sightings hold the latest evidence only.
- **Unknown and pending devices** adds the domain or workgroup and whether Intune knows the device, the two quickest signs of a stranger's device. Segment usage includes VPN pools.
- Audit log filters now offer every action recorded: Stop scan (`scan_stop`), sign-in and sign-out, and the session record type. Saving the Intune connection and Read now are audited like the FortiGate's (`integration` / `intune`); the new scan settings are audited with the rest of System > Settings.
- Segment import accepts the VPN client pool type. Device and register import and export are unchanged: the new details are discovery evidence, not register or device fields, so they come from reports instead.

**Discovery: scanning sized for 1,000 users and 250 servers**
- One steady pace of **30 addresses a second** (System > Settings, 5 to 100), shared by every sweep and the server checks instead of 100 a second per sweep. If 5 pings fail to send, the scan halves its speed until it ends.
- **Empty addresses cost one ping.** An address that ignores ping only gets NetBIOS and the TCP presence check (445, 3389, 22) on server networks, where something was seen in the last 30 days or it is registered, or on one sweep an hour per segment (and on Scan now). Before, every empty address got all of these every scan, most of the traffic.
- **Separate schedules:** user networks every 15 minutes (was 10), server networks every 10 (new setting), monitored servers checked every 5 (was hourly). Only settings still at their old defaults change. Each kind waits out its own interval after a restart.
- **Full identification** is spread by up to an hour per address, so hosts found together aren't identified again together. **Port checks** run every 24 hours per host (new setting, 6 to 168) at most 20 connections a second (was 100).
- System > Settings shows the expected load for your segments: time per round and traffic. The Discovery scanner card shows both schedules and the speed.
- "Seen recently" follows the longer of the two scan intervals (30 minutes with the defaults). Migration 000021 adds the settings, `sightings.ports_checked_at`, and type checks for the new and Intune settings.

**Discovery**
- **VPN users** are now a proper field. The FortiGate read records each SSL and IPsec session's user, VPN kind (and IPsec tunnel) and public origin on its own, instead of inside the hostname (`SSL VPN: nimal from …`). Discovery shows `User: nimal@dialogfinance.lk (SSL VPN from 203.0.113.9)`, search finds VPN users, and a VPN session with a user counts as Known. VPN clients are never asked to be registered. Migration 000020 drops the old VPN sightings, which are read again within one FortiGate interval.
- **VPN client pools** are a new segment type (`vpn`) in the segment form, import, segment lists, dashboard and scan menu. They are never swept: the FortiGate's sessions show who is connected. They have no DHCP pool, environment or port checks, and no "No gateway" warning.
- **Macs and Linux PCs** enrolled in Intune now match by name too: "Nimal's MacBook Pro" in Intune matches `Nimals-MacBook-Pro.local` on the network.
- The SMB handshake ignores Samba's placeholder version (Linux, NAS, macOS file sharing), which would have shown as Windows 7. It still gives the full name and domain.
- Discovery shows the **workgroup or Windows domain** a device names over NetBIOS. The scanner already received it and threw it away. A personal laptop on `WORKGROUP` now stands out from company PCs.
- Discovery shows **Windows version, full DNS name and domain** for Windows hosts that answer file sharing (port 445). During full identification, the scanner starts an SMB2 connection and asks to sign in without credentials, then hangs up at the NTLM challenge, which carries those details. Nobody signs in. Only hosts that answered NetBIOS or have 445 open are asked.
- The Intune read now also brings each device's **serial number, maker and model, OS and version, compliance state, disk encryption and last check-in**, shown in a new "Windows and Intune" section of the finding details. Shared devices with no primary user are now matched too, for their details. Search also finds serial numbers, workgroups and domains. Migration 000019 adds the columns to `sightings`.
- Discovery now shows who uses a device, read from Microsoft Intune. **System > Microsoft Intune** (optional, off by default, permission `system:manage`, audited) reads Intune's managed devices through Microsoft Graph with the sign-in app's own credentials (application permission `DeviceManagementManagedDevices.Read.All`, granted once with admin consent) and matches each device's primary user to sightings by Wi-Fi or Ethernet MAC, or by a computer name only one Intune device has. The user appears in the Discovery row, the finding details and the search. It is evidence only: it never sets an owner. Users of devices that leave Intune are cleared on the next read. Migration 000018 adds the `intune_*` columns to `sightings`.
- DHCP pool addresses are never registered, only static ones. Discovery and the segment address map show them as "DHCP: not registered" with no Register button or menu item; an unapproved device there is still orange, and an approved one counts as Known.
- Registering an address whose device is approved fills in the type, name, platform, owner and department from the device, and links the entry to it (anything typed by hand stays). Without an approved device, the type is guessed from the evidence (camera, printer, access point, network device, PC).
- Editing a register entry no longer drops its link to the device: the form never sent `deviceId`, so each save cleared it.
- Devices with a recorded owner are listed as "Owner › name" in Discovery, Devices and the segment address map.
- The register form's monitoring hint said checks run every minute; they follow the server check schedule (hourly by default).
- Device types are recognised more often. Port checks now include camera ports (554, 8000, 37777), Google Cast (8008, 8009) and Apple's device sync port (62078, iPhone and iPad), and the icon and details read them. Network cards from makers that build whole devices name the device: cameras (Hikvision, Dahua, Uniview, Axis), and network gear (Huawei enterprise, HPE, Aruba, H3C, Cisco, Ubiquiti, MikroTik, Fortinet). UPnP Wi-Fi access points show as such.
- NetBIOS name queries now get answers from Windows PCs on other networks. The scanner asked for the wildcard name padded with spaces and flagged as a broadcast; Windows ignores that. It now sends the query exactly as `nbtstat -A` does (`*` padded with zero bytes, unicast). On routed networks such as other floors and Wi-Fi, Windows PCs now show their computer name and network card MAC without reverse DNS or the FortiGate.
- NetBIOS replies are now read correctly. A reply has no question section, but the reader always skipped one, so every real Windows reply was discarded as invalid and no name or MAC came from NetBIOS. A test now reads a Windows-style reply.
- UPnP descriptions declared as ISO-8859-1 (common on printers and routers) are now read. Go's XML reader refused them, so those devices showed no maker or model.
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
