# Firewall

The **Firewall** section of the menu is DF-IPAM's view of the connected FortiGate. DF-IPAM **only reads** the firewall, with GET requests over its REST API, and never changes it. The connection (address, read-only API token, VDOM, certificate check) is set in **System > FortiGate** (`system:manage`).

The API token's admin profile needs read access to: System (interfaces, DHCP servers, status and resources), Network (routes), VPN (IPsec, SSL VPN), Firewall (addresses and virtual IPs), and User & Device (device identification). A part the profile can't read is named on screen, and the rest still works.

## Three ways the firewall is read

| | What | When | Stored |
| --- | --- | --- | --- |
| Scheduled read | Address table (ARP), DHCP leases, VPN sessions | Every N minutes, when switched on in System > FortiGate | Yes, as Discovery evidence (sightings), which raises alerts and feeds reports |
| Live | Interfaces and traffic, IPsec tunnels, VPN users, SD-WAN health, CPU, memory, sessions | Every 10 seconds while someone watches | No |
| Review and import | WAN lines and tunnels, networks, devices, servers | When someone presses Fetch | Only what people confirm |

Live doesn't replace the scheduled read: Discovery's alerts (unknown devices, conflicts, unregistered addresses) need evidence kept over time, and Live keeps nothing.

## Live

`/firewall/live` (`inventory:view` or `links:view`), read at most once every 10 seconds however many people watch:

- **Interfaces:** state, address, role, port speed, errors, and download and upload rates worked out from the byte counters, with a bar against the link's bandwidth or the port speed. WAN and recorded interfaces show by default.
- **VPN users:** everyone connected over SSL VPN or dial-up IPsec, with their tunnel address and where they connected from.
- **IPsec tunnels:** phase 2 selectors up, and traffic in and out.
- **SD-WAN health:** latency, jitter and packet loss per health check and member.
- **Load:** CPU, memory and sessions.

People who may see network links also see which link each interface and tunnel belongs to, and a link's page shows a **Live from the firewall** card.

## Review and import

`/firewall/review`: fetch, look, tick, confirm. Nothing is saved before the confirmation, which lists every change. One fetch serves the Networks, Devices and Servers tabs; Network links has its own.

| Tab | From the firewall | Becomes | Needs |
| --- | --- | --- | --- |
| Networks | LAN, DMZ and VLAN interfaces with a static address (network, gateway, VLAN, DHCP pool); SSL VPN tunnel pools and dial-up IPsec address ranges | Segments (DHCP, server, cloud or VPN client pool). A recorded segment shows its differences (gateway, VLAN, DHCP pool), which can be applied. Overlapping networks and sizes outside /16 to /30 can't be added | `network:manage` |
| Devices | Every MAC in the address table, DHCP leases and the firewall's device identification, with IP, hostname, system, vendor and kind | Approved devices, with a name, type and platform suggested from what the firewall identified. Devices already decided show their decision | `inventory:edit` |
| Servers | Single-host address objects, inside addresses of virtual IPs, and DHCP reservations (with their MAC) | Register entries in the segment that holds them, with the object's name as Assigned to, a type (Server on server and cloud segments) and optional monitoring. Already registered addresses and addresses outside any segment are shown but can't be ticked | `inventory:edit` |
| Network links | WAN-role interfaces and IPsec tunnels | Network links, or updates to recorded ones; tunnels are linked to the line they run over. Saved after the server reads the firewall again | `links:create`, `links:edit` |

Networks, devices and servers are saved one at a time through the same routes as the rest of DF-IPAM, so every check, permission and audit record applies; a refused item is listed with why, and the others still save. The firewall is fetched again afterwards.
