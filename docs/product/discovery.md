# Discovery

The backend scans every swept segment every `scan.interval_minutes` (default 10, at least 5), and on **Scan now**. **Discovery is evidence only:** it writes `sightings`, one row per IP and MAC. It never writes the register, devices' decisions or ownership. People decide.

## What the scanner asks

Every question goes to one host's own address and changes nothing on it.

| Protocol | How | What it gets | When |
| --- | --- | --- | --- |
| ICMP ping | One echo per address, over one socket per scan; replies matched by sequence number and sender | Whether it answers; the reply's TTL hints at the system (64 Linux, Android or Apple; 128 Windows; 255 network gear) | Every scan |
| NetBIOS | One status query, UDP 137 | Windows computer name, and often the MAC, even across routers | Every scan |
| TCP 445, 3389, 22 | Connect then close, nothing sent; only to addresses that don't answer ping | Finds Windows PCs and servers that block ping. Only a completed connection counts | Every scan |
| ARP | Reads the server's own ARP table after the sweep. Sends no ARP | IP ↔ MAC where the server shares the network; also hosts that answered nothing else | Every scan |
| Reverse DNS | One lookup | Hostname | Every scan |
| mDNS | Unicast question to UDP 5353 on that host only | Name; model for Apple devices (`MacBookPro18,1`) | Full identification |
| SSDP / UPnP | UDP 1900, then the device's own description over HTTP, from that address only | Maker, model and kind (TV, printer, router, camera) | Full identification |
| TCP connect | About 25 common ports (SSH, HTTP/S, RDP, SMB, databases, printers) | Open ports and their usual service | Full identification, on segments that ask |

**Full identification** happens at most every 6 hours per host, or every 15 minutes on Scan now. When it last happened is stored with the sighting, so a backend restart doesn't identify every host again at once. One that is cut short is retried on the next scan.

## Identifying devices

- **The MAC** comes from the ARP table when the server shares the network, otherwise from NetBIOS. ARP wins when both answer: NetBIOS reports the first network card, which on a host with several (VMs, VPN adapters, docks) is often not the one at that address.
- **A sighting without a MAC** (NetBIOS missed its reply) is joined to the MAC last seen at that address within a day, so the same host isn't listed twice.
- **The computer name** is the NetBIOS workstation (0x00) or server (0x20) name, never another unique name such as a signed-in user's (0x03). A DNS name comes first when there is one.
- **The network card's maker** comes from the MAC's first half, using a built-in copy of the IEEE register (`cmd/ouigen`). It is kept apart from the **device's** maker: an ASUS laptop with an Intel Wi-Fi card shows "ASUS" with "card: Intel". When the device says nothing, its maker is read from its name (ASUS-Q533MJ), and open ports sharpen the guess (RDP means Windows, 9100 means a printer).
- **The icon** follows the most reliable evidence: what a person recorded, the router mark for a gateway, what the device says it is (printer, TV, camera, router), its model, DHCP vendor class or name, its open ports, a specific ping TTL (128 Windows, 255 network gear; 64 is shared by Linux, Android and Apple, so it picks nothing), then the device's maker. A guess shows faded, with its reason on hover.
- **Colours:** red for open alerts and rejected devices; orange for unknown devices and addresses that should be in the register (static, on a server or cloud network, or used by an unapproved device). An approved device on a DHCP pool address stays plain.
- **Platform** is set by people. Forms offer "Use likely platform" from the evidence but never fill it in by themselves.

## Keeping the network steady

| Limit | Value |
| --- | --- |
| Pings | At most 100 a second, 32 workers, a timeout on every probe |
| TCP connections | At most 100 a second across the whole scan, and port checks at most 16 in flight |
| Scans | One at a time. Manual scans at least a minute apart |
| Full identification | At most every 6 hours per host; every 15 minutes on Scan now |
| Restart | No scan until the interval since the last sweep has passed |
| Segment size | More than 4,096 addresses is skipped: split it |
| Retention | Sightings unseen for 90 days, and alerts closed for a year, are removed |

If pings can't be sent (for example the kernel runs out of buffers), the segment's scan status says how many, so incomplete results aren't mistaken for an empty network.

## MAC addresses

A MAC is only visible from the devices' own network. NetBIOS gives MACs for many Windows PCs across routers; for everything else the scanner must share the network. `HOST_NETWORK=true` in `api/.env` runs the backend on the host's network (Linux), so it reads the host's ARP table. The API then listens only on loopback and the Docker gateway. See [setup.md](../operations/setup.md#mac-addresses-and-host-networking).

## FortiGate (optional, read-only)

**System > FortiGate** stores the address (`https://` only), VDOM, TLS verification, read interval and a REST API token, encrypted with `SECRET_ENCRYPTION_KEY` and never returned or logged. DF-IPAM sends only GET requests, reading:

- **The ARP table:** a device seen now.
- **DHCP leases**, with the DHCP vendor class (which hints at the OS). A lease lasts hours or days after a device leaves, so **a lease counts as seen now only when the ARP table confirms it.** Other leases only add their name and vendor class to evidence already held.
- **SSL and IPsec VPN sessions.**

Off by default. "Test connection" and "Read now" work before the schedule is on.
