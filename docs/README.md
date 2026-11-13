# DF-IPAM Documentation

**DF-IPAM Lite** is the Dialog Finance IP address management system: it scans the network, keeps the official IP register, watches servers and raises alerts. It is read-only towards the network.

## Where to start

| You want to | Read |
| --- | --- |
| Know what the system does | [product/overview.md](product/overview.md) |
| Run it, or deploy it | [operations/setup.md](operations/setup.md) |
| Change the code | [engineering/architecture.md](engineering/architecture.md), then the document for the part you touch |
| Know what is built and what isn't | [status.md](status.md) |

## Product: what it does

The specification. What is described here is built.

| Document | Covers |
| --- | --- |
| [overview.md](product/overview.md) | Purpose, scope, words used, data model, questions before go-live |
| [network.md](product/network.md) | Segments, import from text, gateways, environments, departments |
| [discovery.md](product/discovery.md) | What the scanner asks, identifying devices, keeping the network steady, FortiGate |
| [register.md](product/register.md) | IP register, servers and checks, devices, requests and reservations |
| [alerts.md](product/alerts.md) | Alert rules, one alert per cause, notifications |
| [screens.md](product/screens.md) | Screens, reports, settings and roles |

## Engineering: how it is built

| Document | Covers |
| --- | --- |
| [architecture.md](engineering/architecture.md) | Containers, code layout, background loops, tests |
| [database.md](engineering/database.md) | Tables, schema rules, migrations |
| [api.md](engineering/api.md) | REST conventions and limits |
| [authentication.md](engineering/authentication.md) | Sign-in (Microsoft Entra ID, the development sign-in), roles and permissions |
| [frontend.md](engineering/frontend.md) | Design and UI rules |
| [security.md](engineering/security.md) | Threat model and release checklist |

## Operations: running it

| Document | Covers |
| --- | --- |
| [setup.md](operations/setup.md) | Run it on your machine, deploy it to a server, Apache, upgrades |
| [runbooks.md](operations/runbooks.md) | Recovery procedures |

## Project

| Document | Covers |
| --- | --- |
| [status.md](status.md) | What is built, what isn't, what waits on others. Update it when a feature changes status |
| [changelog.md](changelog.md) | What changed, day by day. Add to today's entry when you finish a change |

## Rules that always hold

1. **Read-only towards the network.** Nothing in DF-IPAM changes DHCP, DNS, firewalls or devices. The FortiGate is only read.
2. **Evidence stays apart from the register.** Discovery writes sightings and raises alerts; it never changes the register, ownership or a device's decision. People decide.
3. **Nothing is released or assigned automatically.** A silent address isn't freed, and an expired reservation isn't released.
4. **Every change is audited** in the same transaction as the change.
5. **All data goes through the Go API.** Every table has row level security, and the browser never queries tables.
6. **Permissions, not role names,** are checked, in the API.
7. **No fake data.** Screens show real records, or say what is missing.
