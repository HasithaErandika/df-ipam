# DF-IPAM Documentation

**DF-IPAM Lite** is the Dialog Finance IP address management system: it scans the network, keeps the official IP register, watches servers and raises alerts. It is read-only towards the network.

| Document | Covers |
| --- | --- |
| [new.md](new.md) | **The specification**: what the system does, discovery, alerts, screens, what isn't built |
| [setup.md](setup.md) | **Run it on your machine, and deploy it to a server** |
| [architecture.md](architecture.md) | Containers, code layout, background loops |
| [database.md](database.md) | Tables, schema rules, migrations |
| [api.md](api.md) | REST conventions and limits |
| [authentication.md](authentication.md) | Sign-in (Supabase for now), roles and permissions |
| [frontend.md](frontend.md) | Screens, design and UI rules |
| [security.md](security.md) | Threat model and release checklist |
| [runbooks.md](runbooks.md) | Recovery procedures |
| [development_phase.md](development_phase.md) | What is done, what isn't, and what waits on others |
| [changelog.md](changelog.md) | What changed, day by day |
| [plan.md](plan.md) | Plan, not built: company sign-in in place of Supabase |

## Rules that always hold

1. **Read-only towards the network.** Nothing in DF-IPAM changes DHCP, DNS, firewalls or devices. The FortiGate is only read.
2. **Evidence stays apart from the register.** Discovery writes sightings and raises alerts; it never changes the register, ownership or a device's decision. People decide.
3. **Nothing is released or assigned automatically.** A silent address isn't freed, and an expired reservation isn't released.
4. **Every change is audited** in the same transaction as the change.
5. **All data goes through the Go API.** Every table has row level security, and the browser never queries tables.
6. **Permissions, not role names,** are checked, in the API.
7. **No fake data.** Screens show real records, or say what is missing.
