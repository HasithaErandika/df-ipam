# DF-IPAM Lite

**Branch:** `lite`. On this branch this file is the specification: it overrides every other document. What is listed here is built; what isn't built is listed under [Not built](#not-built).

DF-IPAM Lite is where Dialog Finance finds out what is on its network and keeps the official record of its IP addresses. It watches, records and reports. **It never changes the network:** it doesn't configure DHCP, set IPs on machines, block devices or touch the firewall. When an IP is assigned in DF-IPAM, that is the official record, and an engineer sets the IP on the machine.

## The network it serves

| Part | What it is |
| --- | --- |
| Floors | One building, 7 floors. Each floor has a user segment in `192.168.x.x`, served by DHCP. Some users set a static IP by hand |
| Servers | `172.x.x.x`, static IPs only, split by environment (Production, UAT and others). Tracked and watched |
| Cloud | Networks hosted in a cloud: static like servers, never swept from the building |
| Firewall | The FortiGate can be read, read-only and optionally |

IPv4 only. No agents on PCs: the DF-IPAM server is the only thing that runs all the time.

> **Check the server range.** `172.168.x.x` is a public internet range. Private is `172.16.0.0` to `172.31.255.255`. Use the exact range when setting up segments.

## Segments

A segment is one network: name, type, floor, VLAN, CIDR, gateway and description.

| Type | Has | Discovery |
| --- | --- | --- |
| `dhcp` (DHCP network: a floor or Wi-Fi) | A DHCP pool, and optionally a floor number. An address outside the pool is static | Swept |
| `server` | A default environment for new entries | Swept |
| `cloud` | A default environment | Never swept |

Per segment, off by default:

- **Alert on devices with a private MAC.** Phones and laptops use random MACs on Wi-Fi, so these are not unknown-device alerts unless this is on.
- **Check common ports on each device.** See [Discovery](#discovery).

Networks never overlap, and a segment can't be moved away from the register entries and reservations inside it.

**Import from text.** Network setup > Segments > Import from text takes FortiGate `show system interface` output (from either firewall prompt, `$` or `#`), or one network per line such as `Floor 3 192.168.3.0/24`. Each interface address becomes the segment's gateway, `vlan_60`-style names give the VLAN, and the firewall's name goes in the description. Tunnels, loopbacks, /31 and /32 addresses, networks that already exist or overlap one, and repeats are skipped with the reason. A name already used gets the firewall's name added. People review every row, untick any to leave out, then add; nothing is saved before that.

## Discovery

The backend scans every swept segment every `scan.interval_minutes` (default 10, at least 5). What it asks each address:

| Protocol | How | What it gets | When |
| --- | --- | --- | --- |
| ICMP ping | One echo per address | Whether it answers; the reply's TTL hints at the system (64 Linux, Android or Apple; 128 Windows; 255 network gear) | Every scan |
| ARP | Reads the server's own ARP table after pinging. Sends no ARP | IP ↔ MAC, when the server shares the network (see [MAC addresses](#mac-addresses)) | Every scan |
| Reverse DNS | One lookup | Hostname | Every scan |
| NetBIOS | One status query, UDP 137 | Windows name, and often the MAC, even across routers | Every scan |
| mDNS | Unicast question to UDP 5353 on that host only | Name; model for Apple devices (`MacBookPro18,1`) | At most every 6 hours, and on Scan now |
| SSDP / UPnP | UDP 1900, then the device's own description over HTTP, from that address only | Maker, model and kind (TV, printer, router, camera) | At most every 6 hours, and on Scan now |
| TCP connect | About 25 common ports (SSH, HTTP/S, RDP, SMB, databases, printers), connect then close, nothing sent | Open ports and their usual service | Only on segments that ask; at most every 6 hours, and on Scan now |

The maker of the **network card** comes from the MAC's first half, using a built-in copy of the IEEE register (`cmd/ouigen`). It is kept apart from the **device's** maker: an ASUS laptop with an Intel Wi-Fi card shows "ASUS" with "card: Intel". When the device says nothing, its maker is read from its name (ASUS-Q533MJ), and open ports sharpen the guess (RDP means Windows, 9100 means a printer).

Gateways are flagged from the segment setting, the scanner's default route, or the resolver's `_gateway` name. A detected gateway can be set on the segment with one click.

**Evidence only.** Discovery writes `sightings`, one row per IP and MAC. It never writes the register, devices' decisions or ownership. People decide.

### Keeping the network steady

- At most 100 pings a second, 32 workers, a timeout on every probe.
- One scan at a time. Manual scans are at least a minute apart.
- Port checks share 16 connections in flight across the whole scan.
- Slow questions (mDNS, UPnP, ports) go to a host at most every 6 hours, or on Scan now; their results are kept in between.
- A backend restart doesn't scan until the interval since the last sweep has passed.
- Segments with more than 4,096 addresses are skipped: split them.
- Sightings unseen for 90 days are removed.

### MAC addresses

A MAC is only visible from the devices' own network. NetBIOS gives MACs for many Windows PCs across routers; for everything else the scanner must share the network. `HOST_NETWORK=true` in `api/.env` runs the backend on the host's network (Linux), so it reads the host's ARP table. The API then listens only on loopback and the Docker gateway.

### FortiGate (optional, read-only)

System > FortiGate stores the address (`https://` only), VDOM, TLS verification, read interval and a REST API token, encrypted with `SECRET_ENCRYPTION_KEY` and never returned or logged. DF-IPAM sends only GET requests, reading the ARP table, DHCP leases (with the DHCP vendor class, which hints at the OS) and SSL and IPsec VPN sessions. Off by default. "Test connection" and "Read now" work before the schedule is on.

## The IP register

The register is the official list of addresses: IP (unique), segment, status, type, environment, assigned name or device, expected MAC, platform, services, owner, department, monitor, critical, check port and notes.

| Status | Meaning |
| --- | --- |
| Assigned | In use by what it names |
| Reserved | Kept for later |
| Blocked | Never use |

Ways in:

- **From Discovery:** an address seen but not registered has **Register**, pre-filled from what was seen. Approving a device can register its address in the same step.
- **By hand:** the form suggests the next **Available candidate**: not registered, not reserved, outside the DHCP pool, not the gateway, and not seen recently. The API rejects an IP already registered and warns if it was seen in use.
- **Import** a CSV, text (`.txt`, `.tsv`: comma, tab, semicolon or pipe) or Excel file (5 MB, 5,000 rows) with the same columns as the export, so an export can be edited and imported back. On the Servers tab a blank type means Server, and blank Monitor and Critical take the environment's defaults.

**Every import is reviewed first.** Choosing a file runs a dry run, which does the whole import in a transaction that is rolled back, and shows exactly which rows would be added and which skipped and why. Only Import, after that review, saves anything.
- **Release** an entry, after a confirmation. Audited.

Platform and services are set by people. Forms offer "Use likely platform" from the evidence but never fill it in by themselves.

## Servers and environments

The Servers tab is the register's server and cloud entries plus anything monitored, filtered by environment.

| Environment | Severity when down or MAC changes | New servers |
| --- | --- | --- |
| Production | Critical | Monitored, critical |
| DR | Critical | Monitored, critical |
| UAT | High | Monitored |
| Development, Test | Medium | Not monitored |

Environments are added or changed in Network setup. A server can be marked critical by hand.

Monitored entries are checked every `server_check.interval_seconds` (default once a day): ping, then the check port when ping is blocked. A server is **Down** after `server_check.down_after_failures` failures in a row (default 3).

## Devices

Known MAC addresses: name, owner, department, type, platform and status (`pending`, `approved`, `rejected`). A MAC seen for the first time becomes **pending**; private MACs do so only on segments that watch them. Approving can register the address it was seen on. Devices import and export as CSV or Excel.

**Departments** come from a list in Network setup. A rename carries over to devices and register entries in the same audited change; a department in use can't be deleted.

## Alerts

The rules run after every scan, server check and FortiGate read, and after every change to devices, the register or segments, so a warning closes as soon as its cause is gone.

| Alert | When | Severity |
| --- | --- | --- |
| Unknown device | A real (not private) MAC that isn't approved | Medium on floors, High on server and cloud |
| Private-MAC device | A private MAC that isn't approved, on a segment that watches them (never the gateway) | Medium on floors, High elsewhere |
| Rejected device | A rejected MAC is active | High |
| Unregistered static IP | An address outside the DHCP pool is in use and not registered (never the gateway) | Medium |
| Unknown server IP | An unregistered address answers in a server or cloud segment (never the gateway) | High, Critical where the environment is |
| Registered IP misused | A Reserved or Blocked entry, or a reservation, is in use; or a floor entry is used by another MAC than expected | High |
| IP conflict | One IP seen with more than one MAC | High |
| Server MAC changed | A server answers with another MAC than expected | From its environment, Critical if critical |
| Server down | A monitored server is Down | From its environment, Critical if critical |

Alerts can be acknowledged, and close when the cause is gone. One open alert per cause. A cause that closes and reopens within an hour reopens quietly. More than five new High or Critical alerts at once become one summary notification.

**Registered devices on another network.** The register is per address. An approved device seen at an unregistered address whose MAC is registered elsewhere shows "Registered elsewhere: at 192.168.1.7 · Wi-Fi" and counts as known. It raises nothing on a floor DHCP pool; in a server segment the unknown-server-IP alert still fires. A phone with a private MAC uses another MAC on each network, so it can't be linked.

## Requests and reservations

**IP requests.** A Requester asks for an IP: segment, type, name, purpose, needed-by date and optionally a specific IP (at most 20 pending each). An Admin approves with the suggested Available candidate or another IP, or rejects with a reason. Approval creates the register entry in the same transaction and notifies the requester. States: `pending`, `approved`, `rejected`, `cancelled`.

**Reservations.** Hold an IP or a range of up to 256 addresses, with a reason, owner and optional expiry. The owner is reminded once in the 7 days before expiry. Nothing is released automatically: the owner or an Admin extends, converts part of it to an assignment, or releases it. A reservation overlapping a registered, recently seen or reserved address is refused.

## Notifications

A bell in the top bar shows each person's notifications and, for those who see the network, how many alerts are open. Every new alert is a notification in the app; High and Critical ones also go by email when switched on in System (off by default, and needs `SMTP_HOST`). Sent for High and Critical alerts, new requests (to those who approve them), request decisions, servers back up and expiring reservations. Each person can switch email off per type. At most 20 emails an hour per person; the rest stay in the app. Read notifications are removed after 90 days.

## Reports

On demand, as CSV, Excel or PDF (PDF up to 5,000 rows, the others 200,000): Segment usage, Servers by environment, IP register, Unknown and pending devices, Static IPs on floors, Server availability, Alert history, Requests and reservations, and Audit log.

## Screens

| Screen | Holds |
| --- | --- |
| Dashboard | Headline figures; every network by type with the most active ones; latest alerts; DHCP pool use; servers by environment, the register and devices |
| Discovery | Findings with counts that filter (Needs action, Open alerts, Undecided devices, Known, New in the last hour), a details dialog per address, Segments overview and Scan status. Each segment has a page with an address map and free ranges |
| Servers | Servers by environment with live status, and the full IP register, with what answers at each address; click one for every detail |
| Devices | Approved, pending and rejected devices, with what the scanner knows about each; click one for every detail and its decisions |
| Alerts | Open and recent alerts, acknowledge |
| IP Requests | Raise, and for Admins the approval queue |
| Reservations | Active and expiring holds |
| Reports | Pick a report, range and format |
| Audit log | Every change, with filters and export |
| Network setup | Segments (with import from text), environments, departments |
| Users and roles | People and their roles; roles and their permissions |
| System | Intervals, email, sessions, and the FortiGate connection |

## System settings

People who may manage the system change these in System > Settings: scan interval (5 to 1,440 minutes), server check interval, failed checks before Down, reservation reminder days, idle sign-out (default 30 minutes, with a warning one minute before), and the email switch. Every edit on the administration pages asks first, lists exactly what changes, and confirms when saved.

## Roles

A role is a job and the permissions it grants; each person holds one. People who may manage users set roles up in **Users and roles > Roles**: add a role (optionally copying another's permissions), rename it, change its permissions, or delete one nobody holds. Changes apply on each holder's next request.

| Starting role | Grants |
| --- | --- |
| Administrator | Every permission, including ones added later. A system role: it can be renamed but not limited or deleted |
| Network engineer | See and change the register and devices, set up the network, start scans, request and reserve, approve requests, handle alerts, export reports |
| IT officer | See and change the register and devices, request and reserve, handle alerts, export reports |
| Security auditor | See the network, read the audit log, export reports |
| Requester | See the network, request and reserve, export reports |
| Viewer | See the network, export reports |

People sign in with their **Microsoft** `@dialogfinance.lk` account (Entra ID), or, until it is removed, Supabase email and password. A signed-in person with no role sees nothing. Nobody changes their own role or access, and no change may leave nobody active who can manage users. Notifications go by permission: alerts and servers back up to those who handle alerts, new requests to those who approve them. The code checks permissions, never role names. See [authentication.md](authentication.md).

## Data model

Sixteen tables. See [database.md](database.md).

`users`, `roles`, `sessions`, `segments`, `environments`, `departments`, `devices`, `ip_addresses`, `reservations`, `ip_requests`, `sightings`, `alerts`, `notifications`, `notification_preferences`, `settings`, `audit_logs`.

## Not built

| Item | Why it matters | Needs |
| --- | --- | --- |
| ARP sweep on each floor VLAN from a trunk port | Every MAC, even with firewalls on | A trunk port with the floor VLANs for the DF-IPAM server |
| SNMP read of the core switches (ARP, MAC tables, LLDP and CDP neighbours) | MACs across routed networks, and the switch port of each device | Read-only SNMP v2c or v3 on the switches |
| Reading the floor DHCP server directly | Exact DHCP or static, without relying on the pool range | Read access to the DHCP server. The FortiGate leases cover it if it serves DHCP |
| Removing Supabase sign-in | One way in; no external sign-in service | Your go-ahead. See [plan.md](plan.md) |

## Questions to answer first

| # | Question | Why |
| --- | --- | --- |
| 1 | Every segment: floor, VLAN, CIDR, DHCP pool, and each server segment's environment | Setup, and static detection depends on the pool |
| 2 | Is the server range `172.168.x.x`, or within `172.16`–`172.31`? | See the warning at the top |
| 3 | Can the DF-IPAM server have a trunk port with the floor VLANs, or read-only SNMP on the core switch? | MAC-based detection everywhere |
| 4 | Which device serves DHCP to the floors? | Exact static detection |
| 5 | Can the server ping every floor and server segment? | Discovery and server checks |
| 6 | Is Wi-Fi on the floor segments or separate? Guest Wi-Fi? | Guest Wi-Fi should not raise unknown-device alerts |
| 7 | Who gets alert emails, and who approves IP requests? | The Admin list |
| 8 | Is there a device list (asset register, AD computers) to import? | Avoids hundreds of unknown devices on day one |
| 9 | May common ports be checked on company segments? | Security tools may see it as a port scan |
