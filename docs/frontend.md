# Frontend

**DF-IPAM** · Dialog Finance – IP Address Management

## Stack

| Need | Choice |
| --- | --- |
| Base | React, TypeScript, Vite, Tailwind CSS |
| Components | Radix primitives with the project's own components in `components/ui` |
| Routing | React Router |
| API state | TanStack Query |
| Tables | Plain tables with server-side paging |
| Forms | `useChecks` with `lib/validation.ts`. React Hook Form with Zod on sign-in and new subnet |
| Charts | Hand-built SVG |
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

Device types use network-diagram symbols drawn for this project, in `web/src/components/deviceIcons.tsx`, on tiles coloured by kind. Windows, Linux, macOS, iOS and Android use their real logos, in `web/src/components/osLogos.tsx`. Network OS and Hypervisor stay generic, because they cover many vendors. Logos only identify the system a record runs, and belong to their owners.

## Dialogs

Every dialog is centred over a blurred background, using `Modal` and `ConfirmDialog` in `web/src/components/ui/`. Don't add side drawers.

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
| 17 | System Administration | System health, queues and retention. Sites, VLANs and VDOMs are under Network Management, and imports have their own page. SSO configuration is deferred |

## Reports

| Group | Reports |
| --- | --- |
| Registers | IP allocation register, subnet register, device inventory |
| Capacity | Unused address candidates, subnet utilisation and trends |
| Discovery | Discovery evidence, discovery coverage, unmanaged devices |
| Risk | Conflict investigation, ownership gaps and network ownership |
| Governance | Change history, audit trail, integration reports |

Formats: Excel, CSV and PDF.

## Keeping it simple

Show what most people need, and put the rest one click away.

- The sidebar hides what a person can't use.
- Long forms show the essentials, with the rest under `MoreOptions`. It opens by itself if a hidden field has an error.
- Table rows have one main action, and a menu for the others.

## Forms

- **Checks:** `web/src/lib/validation.ts` holds the shared checks, and `useChecks` shows them from the first attempt to save. The API checks everything again and has the final word.
- **Reference data:** pick it with `SearchSelect`, or `RefSelect`, `RoutingDomainSelect` and `PersonSelect`. You can type to search, and they show loading and failure states. Short fixed lists, such as status or environment, stay native selects.
- **Messages:** `Field` links each error or hint to its control for screen readers.
- **Dates and times:** use the browser's pickers. Date and time values are sent in UTC.

## Global features

Left sidebar, top bar, global search, breadcrumbs, user menu, notification centre, saved filters, pagination, saved views, sorting, column visibility, export controls, confirmation dialogs and keyboard navigation.

Every data view has loading, empty and error states.

## Large data

- Tables always use server-side filtering, sorting and pagination, through `usePaging` and `Pagination`. Screens show 20 rows by default, with 50, 100 or 200 in the selector; the API default stays 50 for callers that send no size. Page and page size live in the URL, so they survive refresh and the back button.
- Dropdowns load every option through `listAll`, never one capped page.
- The interactive subnet grid handles small and large networks. It is colour-coded by allocation state, and loads ranges through `GET /subnets/:id/addresses?offset=&limit=`.
- An IPv6 subnet opens on its recorded addresses plus a jump-to-address box, never a full listing.
- Address counts arrive as strings and are formatted without converting to a JS number.

## Dashboard metrics

**Totals:** IPv4 and IPv6 subnets, recent alerts, discovery health, tracked IPs, allocated, reserved, available candidates, observed active, unknown IPs, suspected conflicts, high-utilisation subnets, devices discovered, discovery job status, integration health, pending approvals, recent changes.

**Charts:** subnet utilisation, IPv4/IPv6 distribution, allocation states, environments, sites, discovery trends, conflict trends.

Every summary links to the matching filtered list.

## Accessibility

Target WCAG 2.2 AA: full keyboard use, visible focus, labelled form fields, errors linked to their fields, and sufficient contrast.

## Code layout and addresses

- **Layout:** `web/src/app` holds the router and shell, `features/<screen>` holds a screen and the parts only it uses, `components` holds what several screens share, and `lib` holds non-visual code. Keep every file under 200 lines.
- **Loading:** each screen is loaded the first time it opens (`app/router.tsx`). Libraries sit in their own chunks, so browsers keep them across deploys.
- **Tabs are paths:** a page's tabs use `/reconciliation/changes`, through `RouteTabs` and `useRouteTab`. The page stays one component, so its header and shared state survive a tab change.
- **Query strings are for view state:** filters, sorting, paging and `?id=` for an open dialog. Tabs inside a dialog stay local state.
- **Old links:** `?tab=` addresses in stored notifications and emails redirect to the path, in `AppShell`.

## Feedback and errors

- **Messages:** give each `useMutation` a `meta.success`, and `app/queryClient.ts` shows it as a toast. Add `meta.inlineError` when the screen shows the error itself; otherwise the error appears as a toast.
- **Confirming:** ask before every delete, destructive action and edit save, using `useConfirm` or `useConfirmSave`. Creating a record, and instant switches such as notification preferences, don't ask.
- **Errors:** show `errorMessage(error)`, never a raw exception. A 401 signs the person out, 4xx requests aren't retried, and a missing screen after a deploy asks for a reload.
