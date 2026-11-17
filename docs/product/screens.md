# Screens, Reports, Settings and Roles

## Screens

| Screen | Holds |
| --- | --- |
| Dashboard | Headline figures; every network by type, opening Discovery's segments, with the most active ones; latest alerts; DHCP pool use; servers by environment, the register and devices |
| Discovery | Findings with counts that filter (Needs action, Open alerts, Undecided devices, Known, New in the last hour), a details dialog per address that can be shared as a link, Segments overview and Scan status. Each segment has a page with an address map and free ranges |
| Servers | Servers by environment with live status, and the full IP register, with what answers at each address; click one for every detail |
| Devices | Approved, pending and rejected devices, with what the scanner knows about each; click one for every detail and its decisions |
| Alerts | Open and recent alerts, acknowledge |
| IP Requests | Raise, and for approvers the approval queue |
| Reservations | Active and expiring holds |
| Reports | Pick a report, range and format |
| Audit log | Every change, with filters and export |
| Network setup | Segments (with import from text), environments, departments |
| Users and roles | People and their roles; roles and their permissions |
| System | Intervals, email, sessions, and the FortiGate connection |

Lists refresh on their own (Discovery every 30 seconds, most others every minute), and pause while a dialog is open so rows don't move under it. Details shown as hover hints in tables are all in the details dialog too, for touch and keyboard use.

## Reports

On demand, as CSV, Excel or PDF (PDF up to 5,000 rows, the others 200,000): Segment usage, Servers by environment, IP register, Unknown and pending devices, Static IPs on floors, Server availability, Alert history, Requests and reservations, and Audit log.

## System settings

People who may manage the system change these in **System > Settings**:

| Setting | Default | Range |
| --- | --- | --- |
| Scan interval | 10 minutes | 5 to 1,440 minutes |
| Server check interval | 5 minutes | 1 minute to 1 day |
| Failed checks before Down | 3 | 1 to 20 |
| Reservation reminder | 7 days | 1 to 60 days |
| Idle sign-out | 30 minutes, warned one minute before | 5 to 480 minutes |
| Email | Off | |

Defaults live in the seed (`api/internal/seed`). Every edit on the administration pages asks first, lists exactly what changes, and confirms when saved.

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

People sign in with their **Microsoft** `@dialogfinance.lk` account (Entra ID). A signed-in person with no role sees nothing. Nobody changes their own role or access, and no change may leave nobody active who can manage users. The code checks permissions, never role names. See [authentication.md](../engineering/authentication.md).
