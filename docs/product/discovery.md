# Discovery

The backend scans every swept segment every `scan.interval_minutes` (default 10, at least 5), and on **Scan now**. **Discovery is evidence only:** it writes `sightings`, one row per IP and MAC. It never writes the register, devices' decisions or ownership. People decide.

**Stop** ends a running scan, for anyone who may start one, and is recorded in the audit log. Segments already scanned keep their results and the alert rules still run on them. The segment being scanned saves nothing and isn't counted as a failed scan; the next scan covers it. Server checks are separate and carry on.

## What the scanner asks

Every question goes to one host's own address and changes nothing on it.

| Protocol | How | What it gets | When |
| --- | --- | --- | --- |
| ICMP ping | One echo per address, over one socket per scan; replies matched by sequence number and sender | Whether it answers; the reply's TTL hints at the system (64 Linux, Android or Apple; 128 Windows; 255 network gear) | Every scan |
| NetBIOS | One status query, UDP 137 | Windows computer name, its workgroup or domain, and often the MAC, even across routers | Every scan |
| Windows details (SMB) | TCP 445: SMB2 negotiate, then an NTLM sign-in request with no credentials, closed at the challenge | Windows version and build, full DNS name, domain | Full identification (every 6 hours), for hosts that answered NetBIOS or have 445 open |
| TCP 445, 3389, 22 | Connect then close, nothing sent; only to addresses that don't answer ping | Finds Windows PCs and servers that block ping. Only a completed connection counts | Every scan |
| ARP | Reads the server's own ARP table after the sweep. Sends no ARP | IP ↔ MAC where the server shares the network; also hosts that answered nothing else | Every scan |
| Reverse DNS | One lookup | Hostname | Every scan |
| mDNS | Unicast question to UDP 5353 on that host only | Name; model for Apple devices (`MacBookPro18,1`) | Full identification |
| SSDP / UPnP | UDP 1900, then the device's own description over HTTP, from that address only | Maker, model and kind (TV, printer, router, camera) | Full identification |
| TCP connect | About 30 common ports (SSH, HTTP/S, RDP, SMB, databases, printers, camera video and SDK ports 554, 8000 and 37777, Google Cast 8008 and 8009, Apple's device sync port 62078) | Open ports and their usual service | Full identification, on segments that ask |

**Full identification** happens at most every 6 hours per host, or every 15 minutes on Scan now. When it last happened is stored with the sighting, so a backend restart doesn't identify every host again at once. One that is cut short is retried on the next scan.

## Identifying devices

- **The MAC** comes from the ARP table when the server shares the network, otherwise from NetBIOS. ARP wins when both answer: NetBIOS reports the first network card, which on a host with several (VMs, VPN adapters, docks) is often not the one at that address.
- **A sighting without a MAC** (NetBIOS missed its reply) is joined to the MAC last seen at that address within a day, so the same host isn't listed twice.
- **The workgroup or domain** is the NetBIOS group name of type 0x00. A company PC shows the company's domain; a personal laptop usually shows `WORKGROUP` or a home name, which marks it as a device to look at.
- **Windows details** come from the NTLM challenge a Windows host sends when a file-sharing connection starts: version and build (`10.0.22631` is Windows 11 23H2), full DNS name and DNS domain. Samba (Linux, NAS boxes, macOS file sharing) answers the same way with a placeholder version, which is ignored: it gives the full name and domain only. No credentials are sent and no sign-in is completed, so nothing is logged as a failed logon and nothing changes on the host. A host whose firewall blocks port 445 gives none.
- **The computer name** is the NetBIOS workstation (0x00) or server (0x20) name, never another unique name such as a signed-in user's (0x03). A DNS name comes first when there is one.
- **The network card's maker** comes from the MAC's first half, using a built-in copy of the IEEE register (`cmd/ouigen`). It is kept apart from the **device's** maker: an ASUS laptop with an Intel Wi-Fi card shows "ASUS" with "card: Intel". When the device says nothing, its maker is read from its name (ASUS-Q533MJ), and open ports sharpen the guess (RDP means Windows, 9100 a printer, 554, 8000 or 37777 a camera, 8008 or 8009 a Google Cast TV, 62078 an iPhone or iPad). The card's maker stands for the device's only for makers that build whole devices, not cards for other makers' PCs: camera makers (Hikvision, Dahua, Uniview, Axis) and network gear (HUAWEI TECHNOLOGIES, Hewlett Packard Enterprise, Aruba, H3C, Cisco, Ubiquiti, MikroTik, Fortinet). Plain Hewlett Packard names only the maker, as HP also makes PCs and printers; Huawei's phones register as Huawei Device, and are not counted.
- **The icon** follows the most reliable evidence: what a person recorded, the router mark for a gateway, what the device says it is (printer, TV, camera, router), its model, DHCP vendor class or name, its open ports, a camera maker's network card, a specific ping TTL (128 Windows, 255 network gear; 64 is shared by Linux, Android and Apple, so it picks nothing), the device's maker, then a network gear maker's card. A guess shows faded, with its reason on hover.
- **Colours:** red for open alerts and rejected devices; orange for unknown devices and addresses that should be in the register (static, or on a server or cloud network). **DHCP pool addresses are never registered:** their address changes, so the decision is approving the device. They show "DHCP: not registered" with no Register action, and an approved device there counts as Known. A device with a recorded owner is listed as "Owner › name".
- **Platform** is set by people. Forms offer "Use likely platform" from the evidence but never fill it in by themselves.

## Keeping the network steady

| Limit | Value |
| --- | --- |
| Speed | 30 addresses a second by default (5 to 100), one steady pace shared by every sweep and the server checks, so they never add up to a burst. Halved for the rest of a scan once 5 pings fail to send |
| Schedules | User networks every 15 minutes, server networks every 10, monitored servers checked every 5. Each kind waits out its own interval |
| Empty addresses | An address that ignores ping gets more questions (NetBIOS, then TCP 445, 3389 and 22) only on server networks, if something was seen there in the last 30 days or it is registered, or once an hour per segment, so a new device that blocks ping is still found. Otherwise it costs one ping |
| TCP connections | At most 20 a second across the whole scan, and port checks at most 16 in flight |
| Scans | One at a time. Manual scans at least a minute apart, and they ask every silent address |
| Full identification | Every 6 hours per host, less up to an hour fixed per address so hosts don't all fall due together; every 15 minutes on Scan now |
| Port checks | Where a segment asks for them, every 24 hours per host by default (6 to 168) |
| Restart | Each kind waits until its interval since its last sweep has passed |
| Sizing | About 3,000 addresses (1,000 users and 250 servers) take about 100 seconds a round at 30 a second, under 0.1 Mbit/s. System > Settings shows the estimate for your segments |
| Segment size | More than 4,096 addresses is skipped: split it |
| Retention | Sightings unseen for 90 days, and alerts closed for a year, are removed |

If pings can't be sent (for example the kernel runs out of buffers), the segment's scan status says how many, so incomplete results aren't mistaken for an empty network.

## MAC addresses

A MAC is only visible from the devices' own network. NetBIOS gives MACs for many Windows PCs across routers; for everything else the scanner must share the network. `HOST_NETWORK=true` in `api/.env` runs the backend on the host's network (Linux), so it reads the host's ARP table. The API then listens only on loopback and the Docker gateway. See [setup.md](../operations/setup.md#mac-addresses-and-host-networking).

## FortiGate (optional, read-only)

**System > FortiGate** stores the address (`https://` only), VDOM, TLS verification, read interval and a REST API token, encrypted with `SECRET_ENCRYPTION_KEY` and never returned or logged. DF-IPAM sends only GET requests, reading:

- **The ARP table:** a device seen now.
- **DHCP leases**, with the DHCP vendor class (which hints at the OS). A lease lasts hours or days after a device leaves, so **a lease counts as seen now only when the ARP table confirms it.** Other leases only add their name and vendor class to evidence already held.
- **SSL and IPsec VPN sessions:** the tunnel address, the **user** the FortiGate signed in (an `@dialogfinance.lk` email with Entra SAML sign-in, otherwise the account name), the kind of VPN (with the IPsec tunnel's name) and the public address the session came from. Add the client pool as a **VPN** segment so sessions are grouped there. A VPN client is never asked to be registered; one with a user counts as Known. When an address goes to another session, its user is replaced, not kept.

Off by default. "Test connection" and "Read now" work before the schedule is on.

## Microsoft Intune (optional, read-only)

**System > Microsoft Intune** names the person who uses each device. It reads Intune's managed devices through Microsoft Graph (GET only), with the same Entra app registration and secret as sign-in, so nothing new is stored. The app needs the Graph **application** permission `DeviceManagementManagedDevices.Read.All`, granted once by a tenant administrator with admin consent.

- Macs and Linux PCs enrolled in Intune match too. A Mac called "Nimal's MacBook Pro" in Intune announces itself as `Nimals-MacBook-Pro.local`, so names are compared with apostrophes removed and spaces and punctuation as hyphens. Macs often use a private Wi-Fi address, so the name is usually what matches them.
- Each Intune device (its **primary user**'s sign-in email and display name, and its serial number, maker, model, OS and version, compliance state, disk encryption and last check-in) is matched to sightings by its Wi-Fi or Ethernet MAC, or by computer name when only one Intune device has that name. A MAC match wins over a name match.
- The user is evidence, shown in the Discovery row, the finding details and the search. It never sets a register entry's or device's owner.
- A device that leaves Intune loses these details on the next successful read. Shared devices with no primary user keep their details and show no user.
- A device that isn't in Intune, such as a personal phone or laptop, shows no user. It is still found by the scan and flagged as an unknown device. A network scan can't read who is signed in to a device that DF-IPAM doesn't manage.

Off by default, read every 60 minutes when on (15 to 1440). "Read now" works before the schedule is on.

### Microsoft Entra ID (read with Intune)

Not every company device is enrolled in Intune. Each Intune read also lists Entra ID's devices (joined, hybrid joined or registered) with their **registered owner**, through the same Graph connection (GET only). The app needs two more Graph **application** permissions, `Device.Read.All` and `User.Read.All`, with admin consent. Without them Intune is still read and System > Microsoft Intune shows the Entra ID error on its own.

- Entra ID keeps no MAC addresses, so a device matches only by computer name, compared the same way as Intune names. When Entra holds several copies of a name (a PC joined again), the one that signed in most recently counts. Disabled devices and devices with no owner are skipped.
- Discovery shows the owner as the user when Intune has none (Intune, then Entra ID, then VPN), marked "(Entra ID)", with the Entra name, join type and last sign-in in the finding details. Search finds Entra owners, and the Device users report names the source.
- PCs joined from the office domain (hybrid joined) often have no registered owner in Entra ID, so they give no user this way.
- Evidence only, like Intune: it never sets an owner, and a device that leaves Entra ID loses these details on the next successful read.
