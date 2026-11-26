# Register, Servers and Devices

## The IP register

The register is the official list of addresses: IP (unique), segment, status, type, environment, assigned name or device, expected MAC, platform, services, owner, department, monitor, critical, check port and notes.

The server refuses, however an entry is added (by hand, approving a device, or import): the segment's network and broadcast addresses, DHCP pool addresses (they change, so the device is approved instead), the gateway unless its type is Network, and addresses held by a reservation. The department must be one from the list in Network setup. In an import, Critical with Monitor left blank turns monitoring on.

| Status | Meaning |
| --- | --- |
| Assigned | In use by what it names |
| Reserved | Kept for later |
| Blocked | Never use |

Ways in:

- **From Discovery:** an address seen but not registered has **Register**, pre-filled from what was seen. Approving a device can register its address in the same step.
- **By hand:** the form suggests the next **Available candidate**: not registered, not reserved, outside the DHCP pool, not the gateway, and not seen in use (in the last 30 days outside the DHCP pool, so a device switched off for a weekend keeps its address; 24 hours inside it). The API rejects an IP already registered or held by an active reservation (convert the reservation instead), and warns if it was seen in use.
- **Import** a CSV, text (`.txt`, `.tsv`: comma, tab, semicolon or pipe) or Excel file (5 MB, 5,000 rows) with the same columns as the export, so an export can be edited and imported back. On the Servers tab a blank type means Server, and blank Monitor and Critical take the environment's defaults. Rows for an IP held by an active reservation are skipped and reported.

**Every import is reviewed first.** Choosing a file runs a dry run, which does the whole import in a transaction that is rolled back, and shows exactly which rows would be added and which skipped and why. Only Import, after that review, saves anything.

**Release** an entry, after a confirmation. Audited. Nothing is ever released automatically.

## Servers and environments

The Servers tab is the register's server and cloud entries plus anything monitored, filtered by environment.

| Environment | Severity when down or MAC changes | New servers |
| --- | --- | --- |
| Production | Critical | Monitored, critical |
| DR | Critical | Monitored, critical |
| UAT | High | Monitored |
| Development, Test | Medium | Not monitored |

A server can be marked critical by hand.

### Server checks

Monitored entries are checked every `server_check.interval_seconds` (default 1 hour): two pings, so one lost packet isn't a failure, then the check port when ping is blocked. A server is **Down** after `server_check.down_after_failures` failed checks in a row (default 3, so about 3 hours), and **Up** again on the first good check. Each check also records how fast the server answered (the faster of two pings, or the check port's connect time), shown next to its status; slower than the limit in Settings raises a **Slow server** alert. All results of one round are recorded in one update.

## Devices

Known MAC addresses: name, owner, department, type, platform and status (`pending`, `approved`, `rejected`).

Approving a device or registering an address from Discovery starts with what discovery found already filled in: the name, the user (Intune, then Entra ID, then VPN) as owner, the likely platform and type, and for a register entry a note with maker and model, serial, OS and domain. It is a starting point the person checks; nothing is saved until they do, and an approved device's own record replaces these guesses in the register form.

- A MAC seen for the first time becomes **pending**; private MACs do so only on segments that watch them.
- Approving can register the address it was seen on. Rejecting only marks it: DF-IPAM never blocks anything, but a rejected MAC raises a High alert whenever it is seen.
- Devices import as CSV, text or Excel, and export as CSV, tab-separated text (.txt) or Excel. A text export imports back as is.

**Registered devices on another network.** The register is per address. An approved device seen at an unregistered address whose MAC is registered elsewhere shows "Registered elsewhere: at 192.168.1.7 · Wi-Fi" and counts as known. It raises nothing on a floor DHCP pool; in a server segment the unknown-server-IP alert still fires. A phone with a private MAC uses another MAC on each network, so it can't be linked.

## Requests and reservations

**IP requests.** A Requester asks for an IP: segment, type, name, purpose, needed-by date and optionally a specific IP (at most 20 pending each). An approver approves with the suggested Available candidate or another IP, or rejects with a reason. Approval creates the register entry in the same transaction and notifies the requester. States: `pending`, `approved`, `rejected`, `cancelled`.

**Reservations.** Hold an IP or a range of up to 256 addresses, with a reason, owner and optional expiry. The owner is reminded once in the 7 days before expiry. Nothing is released automatically: the owner or an Admin extends or releases it. Converting part of it to an assignment needs `inventory:edit` or `requests:approve` (an approver converts only their own holds), so a Requester gets an address through an IP request, never by reserving and converting. A reservation overlapping a registered, recently seen or reserved address, the network or broadcast address, the gateway or the DHCP pool is refused. An expiry date ends at midnight in Sri Lanka.

Dates such as "needed by" and "expires" are checked against the person's own calendar day.
