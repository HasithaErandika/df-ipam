# Network Setup

Segments, environments and departments are set up in **Network setup** by people who may manage the network. Every change asks first, lists what changes, and is audited.

## Segments

A segment is one network: name, type, floor, VLAN, CIDR, gateway and description.

| Type | Has | Discovery |
| --- | --- | --- |
| `dhcp` (a floor or Wi-Fi) | A DHCP pool, and optionally a floor number. An address outside the pool is static | Swept |
| `server` | A default environment for new entries | Swept |
| `cloud` | A default environment | Never swept. The FortiGate's ARP table still reports on it, so WAN hand-offs and provider subnets show what the firewall sees. The type for WAN, provider and VPN tunnel addresses |
| `vpn` (a VPN client pool) | Nothing more: the addresses the FortiGate gives SSL and IPsec VPN users | Never swept. The FortiGate's VPN sessions show who is connected; addresses are never registered, as they change each session |

Per segment, off by default:

- **Alert on devices with a private MAC.** Phones and laptops use random MACs on Wi-Fi, so these are not unknown-device alerts unless this is on.
- **Check common ports on each device.** See [discovery.md](discovery.md#what-the-scanner-asks).

### Rules

- Networks are IPv4, `/16` to `/32`, written as their network address, and never overlap. A `/31` or `/32` is a point-to-point hand-off or a single address (such as a WAN IP): every address in it is usable, and a `/32` has no gateway or DHCP pool. Text import keeps `/31` and `/32` lines too.
- The gateway and the DHCP pool use host addresses: never the network or broadcast address. The gateway may sit inside the pool, where the DHCP server leases the whole range and excludes its own address.
- A segment can't be moved away from the register entries and reservations inside it, and only an empty segment can be deleted.
- **Only private networks are swept:** 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16 and 100.64.0.0/10. A floor or server segment with a public range is never swept, and says "Public range: not swept"; use the Cloud type for public addresses.
- **A segment's DHCP pool and gateway follow the register:** an edit can't move the pool over register entries, nor the gateway onto an entry that isn't network equipment. A segment that network links use can't be deleted until they're changed.
- **A swept segment with more than 4,096 usable addresses (larger than a /20) is never scanned.** The form warns, and the segment card says "Too large to scan". Split it.
- **A DHCP network without a pool** can't tell static from DHCP addresses, so nothing on it counts as static and it raises no unregistered-static alerts. Its card says "No DHCP pool set".

### Import from text

**Network setup > Segments > Import from text** takes FortiGate `show system interface` output (from either firewall prompt, `$` or `#`), or one network per line such as `Floor 3 192.168.3.0/24`.

- Each interface address becomes the segment's gateway, `vlan_60`-style names give the VLAN, and the firewall's name goes in the description.
- Tunnels, loopbacks, /31 and /32 addresses, networks that already exist or overlap one, and repeats are skipped with the reason. A name already used gets the firewall's name added.
- People review every row, untick any to leave out, then add. Nothing is saved before that.

### Gateways

A gateway is set on the segment, or detected: from the scanner's default route or the resolver's `_gateway` name. A detected gateway can be set on the segment with one click. Gateways never raise unregistered-static or unknown-server alerts.

## Environments

Production, DR, UAT, Development, Test, and any others added here. Each has a colour, a severity for its alerts, and whether new servers in it are monitored and critical by default. See [register.md](register.md#servers-and-environments).

## Departments

A fixed list offered when approving devices and registering addresses. It starts with IT, Finance, Marketing, Cyber Security, Management and HR; Administrators add, rename and delete from there. Devices and register entries, imports included, accept only a department from this list, in its spelling. A rename carries over to devices and register entries in the same audited change. A department in use can't be deleted.
