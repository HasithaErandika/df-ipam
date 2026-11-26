# Screens, Reports, Settings and Roles

## Screens

| Screen | Holds |
| --- | --- |
| Dashboard | A line saying whether anything needs attention (urgent alerts, servers down, devices to review) or all is clear; headline figures; **Monitor view**; a Network links summary for people who may see links; every network by type, opening Discovery's segments, with the most active ones; latest alerts; DHCP pool use; servers by environment, the register and devices |
| Monitor view | `/monitor`, for a screen on the wall, with the DF-Atlas logo. The top band names each problem (urgent alerts, servers down, lines and tunnels down) or says all systems are normal, with the time and date in Sri Lanka. Tiles: urgent alerts, servers down, unknown devices, devices to review, network links in service, and everything waiting for someone. Panels, all summary charts with no lists of alerts or servers: networks by type (online, registered, usable), open alerts by severity, server health by environment, live WAN traffic over 15 minutes, network links (in service, tunnels up, by status and type, monthly cost for commercial viewers) and DHCP pool use. On a wide screen it fits one screen and never scrolls. It refreshes on its own and has a full-screen button. Each part needs the permission its data does. The idle sign-out pauses while it is open; the 10-hour session limit still applies |
| Discovery | Findings with counts that filter (Needs action, Open alerts, Undecided devices, Known, New in the last hour), a details dialog per address that can be shared as a link, Segments overview and Scan status. Each segment has a page with an address map and free ranges |
| Servers | Servers by environment with live status, and the full IP register, with what answers at each address; click one for every detail |
| Devices | Approved, pending and rejected devices, with what the scanner knows about each; click one for every detail and its decisions |
| Alerts | Open and recent alerts, acknowledge |
| IP Requests | Raise, and for approvers the approval queue |
| Reservations | Active and expiring holds |
| Reports | Pick a report, range and format |
| Audit log | Every change, with filters and export |
| Network Links | Overview, all links, add a link, service providers, contract renewals, import and export. See [network-links.md](network-links.md) |
| Network setup | Segments (with import from text), environments, departments, and the network link types, sites, reminder days and USD rate |
| Users and roles | People and their roles; roles and their permissions |
| System | Intervals, email, sessions, and the FortiGate connection |

Lists refresh on their own (Discovery every 30 seconds, most others every minute), and pause while a dialog is open so rows don't move under it. Details shown as hover hints in tables are all in the details dialog too, for touch and keyboard use.

## Reports

On demand, as CSV, tab-separated text, Excel or PDF (PDF up to 5,000 rows, the others 200,000): Segment usage, Servers by environment, IP register, Unknown and pending devices (with domain or workgroup, and whether Intune knows them), Device users and management (user from Intune or VPN, domain, Windows version, serial, model, OS, compliance, encryption), VPN sessions (the latest session per tunnel address), Static IPs on floors, Server availability, Alert history, Requests and reservations, Audit log, and for people with the links permissions Network links, Network link costs and Network link contract renewals (the last looks ahead: its dates default to the next 90 days). A report needs `reports:export` and the permission of the screen it comes from: `inventory:view` for the network, device and request reports, `audit:view` for the Audit log.

## System settings

People who may manage the system change these in **System > Settings**:

| Setting | Default | Range |
| --- | --- | --- |
| User network scan interval | 15 minutes | 5 to 1,440 minutes |
| Server network scan interval | 10 minutes | 5 to 1,440 minutes |
| Scan speed | 30 addresses a second | 5 to 100 |
| Port check interval | 24 hours | 6 to 168 hours |
| Server check interval | 5 minutes | 1 minute to 1 day |
| Failed checks before Down | 3 | 1 to 20 |
| Slow server limit | 200 ms | 10 to 5,000 ms |
| Scan device segments automatically | On | On or off |
| Scan server segments automatically | On | On or off |
| Check monitored servers automatically | On | On or off |
| Reservation reminder | 7 days | 1 to 60 days |
| Idle sign-out | 30 minutes, warned one minute before | 5 to 480 minutes |
| Email | Off | |
| Team mailbox | None | One email address, or none |
| Email digest | Every 30 minutes | 15 to 240 minutes |
| Daily summary | 08:00 Sri Lanka time | Any hour |

The three automatic switches stop only scheduled work: **Scan now** still scans any segment. While a kind of segment isn't scanned automatically, its alerts stay open; while checks are off, servers keep their last state.

Defaults live in the seed (`api/internal/seed`). Every edit on the administration pages asks first, lists exactly what changes, and confirms when saved.

## Roles

A role is a job and the permissions it grants; each person holds one. People who may manage users set roles up in **Users and roles > Roles**: add a role (optionally copying another's permissions), rename it, change its permissions, or delete one nobody holds. Changes apply on each holder's next request.

| Starting role | Grants |
| --- | --- |
| Administrator | Every permission, including ones added later. A system role: it can be renamed but not limited or deleted |
| Network engineer | See and change the register and devices, set up the network, start scans, request and reserve, approve requests, handle alerts, export reports, and everything in Network Links |
| IT officer | See and change the register and devices, request and reserve, handle alerts, export reports, see network links |
| Security auditor | See the network, read the audit log, export reports, see network links with their costs and contracts |
| Requester | See the network, request and reserve, export reports, see network links |
| Viewer | See the network, export reports, see network links |

People sign in with their **Microsoft** `@dialogfinance.lk` account (Entra ID). A signed-in person with no role sees nothing. Nobody changes their own role or access, and no change may leave nobody active who can manage users. Nobody hands out more than they hold: someone who manages users but isn't an Administrator can't add a permission they lack to a role, change their own role's permissions, give a role holding more than theirs, or change a person who holds more. The code checks permissions, never role names. See [authentication.md](../engineering/authentication.md).
