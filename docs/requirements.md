# Requirements

There are two sources: the main DF-IPAM brief, and a **supplement** numbered §4 to §22 ("core functionality"). The numbers don't line up between them, so references below say which one they mean. This file tracks coverage and does not replace either source.

## Open items

| # | Item | Blocks | Owner |
| --- | --- | --- | --- |
| 1 | **The main brief is cut off at the heading `# 26.`** The supplement covers §4 to §22 of a different numbering, so it may not be the missing part. | Final scope sign-off | Project owner |
| 2 | `Prod_Subnets.txt` and `Prod_VDOMs_Subnets.txt` not yet provided (the supplement says attached, but they were not received) | Validating the import parser | Project owner |
| 3 | Supabase holds the database and sign-in in the cloud. Data residency, connectivity, availability and compliance are not yet approved | Loading production data | Dialog Finance compliance / security |
| 5 | Redis approval (the brief says "where approved") | Worker deployment | Infrastructure |
| 6 | Company auth target and date | Auth migration | Project owner |
| 7 | List of core banking and payment networks | Enabling any active discovery | Network / security |
| 8 | Which DNS and DHCP servers are in use (for example Microsoft, BIND, FortiGate DHCP) | DNS/DHCP connector design | Network / infrastructure |
| 9 | Expected inventory, agent count, discovery frequency and retention | Final server sizing | Network / infrastructure |
| 10 | RPO and RTO targets | Backup design sign-off | Business |

## Deviations from the brief

| Brief | Current decision | Reason |
| --- | --- | --- |
| §18 Microsoft Entra ID, OIDC, PKCE, server-side sessions, Entra group-to-role mapping | **Simple Supabase Auth** for now | Project owner's decision; will move to company auth later. See [authentication.md](authentication.md) |
| §17 `sessions` table | Created but unused | Supabase manages sessions for now; reserved for company auth |
| §24 Optional dark theme (also Supplement §14) | **Light theme only** | Project owner's decision |
| §2 Neon as the primary database | **Supabase PostgreSQL** for now | Project owner's decision. Standard PostgreSQL, so moving later is a connection and data change |
| §19 Secure cookies, CSRF protection (also Supplement §16) | Not applicable while the API uses bearer tokens | Revisit if the company auth uses cookies |
| Supplement §15 Entra SSO, server-side sessions, group-to-role mapping; §13 SSO configuration page | Deferred, same as main §18 | Project owner chose simple Supabase Auth for now. Roles are assigned in DF-IPAM |
| Supplement §21 PHP-FPM, Laravel queue workers, Laravel scheduler, Python services | **Not adopted.** Replaced by `df-ipam-api` and `df-ipam-worker` | Main brief §2 mandates Go. These lines look left over from an earlier stack |
| Supplement §12 Factories and seeders | Go test factories and a `seed` command | Same intent without Laravel |
| Supplement §5 One list of nine statuses | Kept as two fields: allocation state and observation state | The supplement itself requires them to be separate |
| Supplement §4 Environment names "DR", "Test" | Stored as `disaster_recovery`, `testing`; the UI may show the short labels | Same values, one set of stored names |

## Additions to the suggested schema

The brief's table list is a suggestion. These are added because a requirement needs them: `applications`, `teams`, `team_members`, `dns_records`, `dhcp_scopes`, `dhcp_reservations`, `overlap_exceptions`, `subnet_utilisation_snapshots`, `subnet_reserved_ranges`, `routing_domain_peerings`, `cloud_accounts`, `cloud_networks`, `network_routes`, `firewall_address_objects`, `firewall_address_groups`, `firewall_address_group_members`, `agent_registration_tokens`, `discovery_job_logs`, `discovery_blackouts`, `reconciliation_evidence`, `reconciliation_rules`, `approval_workflows`, `approval_workflow_steps`, `notification_rules`, `notification_preferences`, `retention_policies`, `saved_filters`, `system_settings`.

## Traceability

| Brief section | Topic | Documented in |
| --- | --- | --- |
| 1-3 | Overview, stack, architecture | [architecture.md](architecture.md) |
| 4 | Hierarchy | [database.md](database.md) |
| 5 | Subnet management | [database.md](database.md), [frontend.md](frontend.md) |
| 6 | IP address management, Explorer | [database.md](database.md), [frontend.md](frontend.md) |
| 7 | Allocation workflow | [database.md](database.md), [api.md](api.md) |
| 8 | Devices and interfaces | [database.md](database.md) |
| 9-10 | Discovery, scheduling, agents | [discovery-and-reconciliation.md](discovery-and-reconciliation.md) |
| 11-14 | FortiGate, VMware, AWS, Azure | [integrations.md](integrations.md) |
| 15 | Production file import | [integrations.md](integrations.md) |
| 16 | Reconciliation | [discovery-and-reconciliation.md](discovery-and-reconciliation.md) |
| 17 | Database design, ERD, dictionary | [database.md](database.md) |
| 18 | Authentication and roles | [authentication.md](authentication.md) |
| 19 | Security, threat model | [security.md](security.md) |
| 20 | Audit | [database.md](database.md), [security.md](security.md) |
| 21 | Notifications | [architecture.md](architecture.md) |
| 22-23 | Dashboard, reports | [frontend.md](frontend.md), [architecture.md](architecture.md) |
| 24 | Frontend | [frontend.md](frontend.md) |
| 25 | API | [api.md](api.md) |
| 26+ | **Not received** | See open item 1 |

## Supplement traceability

| Supplement | Topic | Documented in |
| --- | --- | --- |
| §4 | Hierarchy, subnets, gateways, DNS, DHCP, teams, overlap exceptions | [database.md](database.md) |
| §5 | Status model and IP record | [database.md](database.md) |
| §6 | Discovery methods and scheduler | [discovery-and-reconciliation.md](discovery-and-reconciliation.md) |
| §7-10 | FortiGate, production files, VMware, AWS, Azure | [integrations.md](integrations.md) |
| §11 | Reconciliation | [discovery-and-reconciliation.md](discovery-and-reconciliation.md) |
| §12 | Database | [database.md](database.md) |
| §13-14 | Pages and UX | [frontend.md](frontend.md) |
| §15 | Auth and roles | [authentication.md](authentication.md) |
| §16-17 | Security, audit, governance | [security.md](security.md), [database.md](database.md) |
| §18-19 | Notifications, reports | [architecture.md](architecture.md), [frontend.md](frontend.md) |
| §20 | Reliability and performance | [architecture.md](architecture.md) |
| §21-22 | Deployment, backup, recovery | [deployment.md](deployment.md) |
