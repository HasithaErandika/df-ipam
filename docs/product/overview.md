# Overview

**DF-IPAM Lite** is where Dialog Finance finds out what is on its network and keeps the official record of its IP addresses. It watches, records and reports. **It never changes the network:** it doesn't configure DHCP, set IPs on machines, block devices or touch the firewall. When an IP is assigned in DF-IPAM, that is the official record, and an engineer sets the IP on the machine.

The product documents in this folder are the specification: what is described here is built. What isn't built is in [status.md](../status.md).

| Document | Covers |
| --- | --- |
| [network.md](network.md) | Segments, import from text, environments, departments |
| [discovery.md](discovery.md) | What the scanner asks, how it identifies devices, how it keeps the network steady, FortiGate |
| [register.md](register.md) | The IP register, servers and their checks, devices, requests and reservations |
| [alerts.md](alerts.md) | Alert rules and notifications |
| [screens.md](screens.md) | Screens, reports, settings and roles |

## Scope

One building, IPv4 only, one routing table. No agents on PCs: the DF-IPAM server is the only thing that runs all the time.

| Part | What it is |
| --- | --- |
| Floors | 7 floors. Each has a user segment in `192.168.x.x`, served by DHCP. Some users set a static IP by hand |
| Servers | `172.x.x.x`, static IPs only, split by environment (Production, UAT and others). Tracked and watched |
| Cloud | Networks hosted in a cloud: static like servers, never swept from the building |
| Firewall | The FortiGate can be read, read-only and optionally |

> **Check the server range.** `172.168.x.x` is a public internet range. Private is `172.16.0.0` to `172.31.255.255`. Use the exact range when setting up segments.

## Words used

| Word | Meaning |
| --- | --- |
| Segment | One network: a floor, Wi-Fi, server or cloud network |
| Sighting | Evidence: one IP and MAC the scanner or firewall saw, with what was learned about it |
| Register | The official list of addresses and what each is for |
| Device | A known MAC address, approved, pending or rejected |
| Available candidate | An address that looks unused: never called "free", because evidence can be wrong |
| Seen recently | Answered within two scan intervals, at least 10 minutes (`seen_window()` in the database) |

## Data model

Sixteen tables: `users`, `roles`, `sessions`, `segments`, `environments`, `departments`, `devices`, `ip_addresses`, `reservations`, `ip_requests`, `sightings`, `alerts`, `notifications`, `notification_preferences`, `settings`, `audit_logs`. See [database.md](../engineering/database.md).

## Questions to answer before go-live

| # | Question | Why |
| --- | --- | --- |
| 1 | Every segment: floor, VLAN, CIDR, DHCP pool, and each server segment's environment | Setup, and static detection depends on the pool |
| 2 | Is the server range `172.168.x.x`, or within `172.16`–`172.31`? | See the warning above |
| 3 | Can the DF-IPAM server have a trunk port with the floor VLANs, or read-only SNMP on the core switch? | MAC-based detection everywhere |
| 4 | Which device serves DHCP to the floors? | Exact static detection |
| 5 | Can the server ping every floor and server segment, and reach TCP 22, 445 and 3389? | Discovery and server checks |
| 6 | Is Wi-Fi on the floor segments or separate? Guest Wi-Fi? | Guest Wi-Fi should not raise unknown-device alerts |
| 7 | Who gets alert emails, and who approves IP requests? | The Admin list |
| 8 | Is there a device list (asset register, AD computers) to import? | Avoids hundreds of unknown devices on day one |
| 9 | May common ports be checked on company segments? | Security tools may see it as a port scan |
