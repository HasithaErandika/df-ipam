# DF-IPAM Documentation

**DF-IPAM** is the Dialog Finance IP Address Management platform: the central system for managing, discovering, auditing and governing IPv4 and IPv6 address infrastructure.

**Status:** Phases 0 to 2 complete: foundation, core IPAM and the allocation workflow. See [development_phase.md](development_phase.md) for what is done and what remains.

| Document | Covers |
| --- | --- |
| [requirements.md](requirements.md) | Scope, spec traceability, deviations, open items |
| [architecture.md](architecture.md) | Services, modules, repository layout, libraries |
| [authentication.md](authentication.md) | Supabase Auth (temporary), roles, moving to company auth |
| [database.md](database.md) | Schema rules, tested constraints, ERD, data dictionary |
| [api.md](api.md) | REST conventions and endpoint map |
| [discovery-and-reconciliation.md](discovery-and-reconciliation.md) | Discovery safety, agents, reconciliation rules |
| [integrations.md](integrations.md) | FortiGate, vCenter, AWS, Azure, file imports |
| [frontend.md](frontend.md) | Pages, design system, UX rules |
| [security.md](security.md) | Threat model and review checklist |
| [deployment.md](deployment.md) | Ubuntu server, Nginx, systemd, Supabase, backups |
| [roadmap.md](roadmap.md) | Delivery phases and exit criteria |
| [development_phase.md](development_phase.md) | **Trace of everything to build, and its current status** |

## Non-negotiable rules

These apply to every module and override convenience.

1. **Allocation and observation are separate.** Discovery never changes allocation state or ownership.
2. **An IP is only unique within a routing domain.** The same private range in two VRFs is not a conflict.
3. **Nothing is released or assigned automatically.** A failed ping does not free an address, and an unused-looking address is not assigned.
4. **All integrations are read-only.**
5. **No discovery outside an approved target scope.** Core banking and payment systems are passive/API-only until explicitly approved.
6. **Raw evidence stays apart from approved inventory.**
7. **Authorisation is enforced in the API and background jobs,** never only in the frontend.
8. **No large range is ever enumerated.** IPv6 subnets use calculated ranges and paginated views.
