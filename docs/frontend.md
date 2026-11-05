# Frontend

**DF-IPAM** · Dialog Finance – IP Address Management

## Stack

| Need | Choice |
| --- | --- |
| Base | React, TypeScript, Vite, Tailwind CSS |
| Components | shadcn/ui on Radix primitives (accessible, source owned by the project) |
| Routing | React Router |
| API state | TanStack Query |
| Tables | TanStack Table with TanStack Virtual |
| Forms | React Hook Form with Zod |
| Charts | Recharts |
| Auth | `supabase-js`, used only in `web/src/lib/auth.ts` |

## Design direction

Professional financial-enterprise interface. Light theme only. Built for desktop and tablet.

- Compact, readable tables with a clear hierarchy
- The dashboard should feel like a mature infrastructure platform, not a basic admin template
- Monospaced type for IPs, MACs and CIDRs
- Restrained colour: no heavy gradients, large cards or decorative animation
- Status is shown with text or icon as well as colour

**Status language is fixed across the app:**

| Group | Values |
| --- | --- |
| Allocation | Allocated, Reserved, Available candidate, Quarantined, Retired |
| Observation | Unknown, Observed active, Observed inactive, Conflict suspected |

Allocation and observation are always shown as two separate badges. "Available candidate" is never shortened to "Free".

**Device types and operating systems** have icons (firewall, router, switch, access point, server, virtual machine, laptop, desktop, phone, tablet, printer, camera and more; Windows, Linux, macOS, iOS, Android, network OS). An OS is shown only when a record or reliable source states it, never guessed.

## Dialogs

Every dialog is centred over a blurred background, using `Modal` and `ConfirmDialog` in `web/src/components/ui.tsx`. Don't add side drawers.

## Pages

| # | Page | Main content |
| --- | --- | --- |
| 1 | Dashboard | Totals, charts, drill-down to records |
| 2 | Network Management | Organisations through VLANs and blocks, with flexible relationships |
| 3 | Subnet Management | Searchable inventory, filters (site, VDOM, VLAN, routing domain, environment), CIDR calculator, hierarchy, overlap validation, discovery settings, bulk actions |
| 4 | IP Address Explorer | Subnet picker, colour-coded grid and table views, search, filters, bulk actions, detail view with evidence and audit history |
| 5 | Device Inventory | Device list and detail: interfaces, MACs, IPs, services, discovery history |
| 6 | Interface Inventory | Cross-device interface search |
| 7 | Discovery Centre | Agents, policies, approved scopes, schedules, manual runs, job history, scan logs, error reports, exclusions, emergency stop |
| 8 | Reconciliation Centre | Findings queue with evidence and review |
| 9 | Integrations | FortiGate, VMware, AWS, Azure, DNS/DHCP: connections, health, sync history |
| 10 | IP Allocation Requests | New request (site/subnet, purpose, business service, owner), approval status, allocation and reservation history |
| 11 | Reservations | Active and expiring reservations |
| 12 | Alerts and Notifications | New devices, conflicts, utilisation, discovery failures, integration errors, stale inventory; rules and preferences |
| 13 | Reports | All report types below, with generation history |
| 14 | Audit Logs | Search and old/new value comparison |
| 15 | User Management | Users and role assignment |
| 16 | Role and Permission Management | Permission matrix |
| 17 | System Administration | Sites, routing domains, VLANs, VDOMs, security policies, notifications, imports and enrichment queue, retention, backup settings, system health. SSO configuration is deferred |

## Reports

| Group | Reports |
| --- | --- |
| Registers | IP allocation register, subnet register, device inventory |
| Capacity | Unused address candidates, subnet utilisation and trends |
| Discovery | Discovery evidence, discovery coverage, unmanaged devices |
| Risk | Conflict investigation, ownership gaps and network ownership |
| Governance | Change history, audit trail, integration reports |

Formats: Excel, CSV and PDF.

## Global features

Left sidebar, top bar, global search, breadcrumbs, user menu, notification centre, saved filters, pagination, saved views, sorting, column visibility, export controls, confirmation dialogs and keyboard navigation.

Every data view has loading, empty and error states.

## Large data

- Tables always use server-side filtering, sorting and pagination, through `usePaging` and `Pagination`. Page and page size live in the URL, so they survive refresh and the back button.
- Dropdowns load every option through `listAll`, never one capped page.
- The interactive subnet grid handles small and large networks. It is colour-coded by allocation state, virtualised, and loads ranges through `GET /subnets/:id/addresses?offset=&limit=`.
- An IPv6 subnet opens on its recorded addresses plus a jump-to-address box, never a full listing.
- Address counts arrive as strings and are formatted without converting to a JS number.

## Dashboard metrics

**Totals:** IPv4 and IPv6 subnets, recent alerts, discovery health, tracked IPs, allocated, reserved, available candidates, observed active, unknown IPs, suspected conflicts, high-utilisation subnets, devices discovered, discovery job status, integration health, pending approvals, recent changes.

**Charts:** subnet utilisation, IPv4/IPv6 distribution, allocation states, environments, sites, discovery trends, conflict trends.

Every summary links to the matching filtered list.

## Accessibility

Target WCAG 2.2 AA: full keyboard use, visible focus, labelled form fields, errors linked to their fields, and sufficient contrast.
