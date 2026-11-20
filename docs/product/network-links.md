# Network Links

**Network Links** is the record of the company's connectivity: internet leased lines, broadband, site-to-site and remote access VPNs, MPLS, leased lines, 4G/5G, AWS Direct Connect, Azure ExpressRoute and third-party connections. People keep it by hand. It is administrative: **nothing is polled, monitored or configured.** A link's status says whether it is in service, not whether it is up right now. Monitoring and service management tools do that.

## Screens

The **Network Links** menu section. Adding a link isn't a menu item: **Add network link** on the Overview and the list opens the form in a dialog.

| Screen | Address | Holds | Needs |
| --- | --- | --- | --- |
| Overview | `/network-links` | Total, active, inactive or terminated, and VPN tunnel counts; monthly recurring cost per currency and contracts ending in 90 days; links by type, provider and location; monthly cost by provider; upcoming renewals; recently updated links | `links:view` (costs and renewals: `links:commercial_view`) |
| All Network Links | `/network-links/all` | The list: Link ID, name, type, provider, source location, then optional columns (destination, bandwidth, role, monthly rental, criticality, circuit ID, public IPs, contract expiry, officer), status and actions. Search across names, Link IDs, circuit IDs, providers, tunnel names, peers, WAN, gateway and public IPs, and remote subnets. Filters by type, provider, location, status, criticality and archived; sort; 10, 25, 50 or 100 rows; export of the filtered list | `links:view` |
| Add network link | `/network-links/new` | The form (below), in a centred dialog over the list. The Overview opens the same dialog in place | `links:create` |
| Link details | `/network-links/<id>` | General, technical, VPN (for VPN types), commercial, support, associated addresses and subnets, primary and backup links, remarks, attachments and change history | `links:view` (history: `audit:view`) |
| Edit | `/network-links/<id>/edit` | The form, filled in, in a dialog over the link's page. Saving lists every change and asks first | `links:edit` |
| Service Providers | `/network-links/providers` | Providers with contacts and link counts; add, edit, switch off | `links:view` (changes: `links:providers`) |
| Contract Renewals | `/network-links/renewals` | Contracts ending within 30, 60, 90 days, 6 months or a year, and those already ended on links still in use, with notice periods and officers | `links:commercial_view` |
| Import / Export | `/network-links/import-export` | Export, the import template, and the import | `links:import_export` |

The segment page lists the links on that subnet (its WAN subnet, a VPN's local subnet, or a WAN or gateway address inside it). A register entry's details list the links using that address (WAN IP, gateway, or inside the public range). Both show only to people with `links:view`.

## A link

The **Link ID** (`DF-LNK-0001`) is given when the link is saved and never changes. Fields marked required must be filled; everything else is optional, because not every link has every detail.

| Section | Fields |
| --- | --- |
| Basic (required: name, type, source location, purpose, role, criticality, status) | Name, link type, service provider, circuit / service ID, source location (a listed site), destination (a site or an external party), business purpose, environment (Production, DR, UAT, Other), link role (Primary, Secondary, Backup, Standalone), criticality (Critical, High, Medium, Low), responsible officer (an active user), status (Active, Inactive, Pending, Terminated) |
| Technical | Download and upload bandwidth (Mbps or Gbps, stored in Mbps), connection medium (Fibre, Wireless, Copper, Other), WAN IP (picked from the register or typed), WAN subnet (a segment), public IP or range (`203.0.113.0/29`), gateway IP, connected device (an approved device, or a name), interface or port, VLAN ID, VDOM / VRF, routing type (Static, BGP, OSPF, Other) |
| VPN, shown only for VPN link types | Tunnel name, VPN type (IPsec, SSL, Other), local and remote peer (IPv4 or a host name), local subnets (segments), remote subnets, remote organisation, remote technical contact, last connectivity test, credential vault reference |
| Commercial | No separate recurring cost (for a VPN paid for through its internet line, so it isn't counted twice), monthly rental and currency (LKR, USD or another code), annual recurring cost (monthly × 12, worked out), one-time installation cost, billing account, contract and purchase order references, activation, contract start and expiry dates, renewal notice period, cost centre |
| Support | Support contact, email and hotline, account manager, agreed SLA availability (%) and resolution time (hours). **Fill in from a service provider** copies a provider's name, support email, hotline and account manager into the fields, which stay editable. Empty fields show the link's provider's own details |
| Related links | Each related link and how: its **secondary** or **backup**, or the **underlying** link a VPN runs over. More than one backup is allowed. A link can't relate to itself, and a pair is stored once: if B already lists A, A can't list B |
| Remarks | Free text |
| Attachments | Files chosen in the dialog, each with its kind, uploaded to the link when it is saved. A file that fails is named, and can be added again from the link's page |

- **Addresses are IPv4.** Remote subnets are the far side's networks: they may overlap other links' remote subnets and are never counted as local address space.
- **No secrets.** There is no field for pre-shared keys or passwords; the vault reference says where they are kept. Configuration files are to be attached with secrets removed.
- A switched-off link type, an inactive provider or a site removed from the list stays on the links that already have it.
- One provider can't have two live links with the same circuit ID.

## Archiving

Links are **archived, never deleted.** An archived link leaves the lists (unless the Archived filter asks for it), the totals and the reminders; it keeps its details, files and history, can't be edited, and can be restored. Providers are switched off instead of deleted.

## Attachments

Added in the link dialog when saving, or later with **Add file** in the link page's Attachments card. Contracts, provider quotations, network diagrams, configuration files and other documents, up to **15 MB** each: PDF, PNG, JPEG, Word, Excel, PowerPoint, Visio, and text (`.txt`, `.csv`, `.cfg`, `.conf`). The content must match the file's name: an HTML page named `.pdf` is refused. Files live on the server's private attachment volume, never in the web server's folders, and are only downloaded through the API, which checks permission each time. **Contracts and quotations** need `links:commercial_view` to open and `links:commercial_manage` to add or remove. Removing a file takes it off the link; the file and its record are kept for the history.

## Money and currencies

Commercial information (rentals, costs, billing, contract references and dates, cost totals, renewals, and contract and quotation files) is shown only to people with `links:commercial_view`, everywhere: lists, details, the overview, exports, change history and files. Without `links:commercial_manage`, the form leaves commercial fields as they are.

Rupees are shown as `Rs. 25,000.00`, other currencies by code (`USD 1,200.00`). **Totals are kept per currency.** When an administrator sets an approved LKR per USD rate, the overview also shows an approximate rupee total. Monthly totals leave out terminated and archived links and links with no separate cost.

## Contract reminders

Every three hours DF-IPAM checks contract expiry dates against the reminder days (90, 60, 30, 7 and 0, the expiry date, by default), in Sri Lanka time. When a link reaches one, its responsible officer and everyone with `links:commercial_manage` get a notification, and an email when email is on and their preference allows (kind "Network link contracts about to expire"). Each reminder goes once per contract end: days already passed are marked together, so a link entered 20 days before expiry gets one notice, not three. A new expiry date starts the reminders again and is recorded as a **contract renewal**. Terminated and archived links get none, nor do contracts that ended more than 7 days ago.

## Import and export

**Export** gives the links matching the list's filters (or all links) as Excel, CSV or text. Excel adds a **VPN subnets** sheet (one row per local or remote subnet) and a **Related links** sheet.

**Import** reads Excel or CSV, up to 2,000 rows and 5 MB, using the **template**'s headers. Its second sheet lists the accepted values. Link types may be given by name or key, providers by name, officers by email, subnets by network or name, devices by name or MAC, related links as `DF-LNK-0002:backup; DF-LNK-0003:underlying`.

1. **Check the file.** A dry run does everything and keeps nothing, listing each row to add or update and each row skipped, with why.
2. **Import.** Rows with problems are skipped; the rest go in, each audited.

A row whose **Link ID** exists, or whose provider and circuit ID match a live link, **updates** that link, but only when **Update existing links** is ticked; otherwise it is skipped as a duplicate. Columns the file leaves out keep their values. An unknown Link ID is refused. New links get the next Link ID.

## Reports

**Reports** has three more, for people who also hold the links permissions: **Network links** (every live link, `links:view`), **Network link costs** (monthly and annual cost per link in its own currency, with billing and contract details, also `links:commercial_view`) and **Network link contract renewals** (contracts ending in a date range that looks ahead, 90 days by default, also `links:commercial_view`). Archived links are left out, and terminated ones from costs and renewals. A report someone may not use isn't listed, and asking for it directly is refused.

## History

Every change is in the audit log in the same transaction: added, changed (each field before and after, related links and local subnets included), contract renewed, archived, restored, file attached and file removed, with who and when. The link's **Change history** shows them field by field to people with `audit:view`, leaving commercial fields out for those who may not see them.

## Master data

**Network setup > Network links** (`network:manage`): the link types (name, whether it is a VPN type, in use or switched off; a type links use can't be removed), the sites links start from, the reminder days, and the approved LKR per USD rate. Changes are confirmed and audited.

## Permissions

| Permission | Allows |
| --- | --- |
| `links:view` | See links, providers, the overview and attachments other than contracts and quotations |
| `links:create` | Add links |
| `links:edit` | Edit links, their related links and attachments |
| `links:archive` | Archive and restore links |
| `links:commercial_view` | See commercial information, renewals and contract and quotation files |
| `links:commercial_manage` | Change commercial information, add contract and quotation files, and receive contract reminders |
| `links:providers` | Add and edit providers |
| `links:import_export` | Export, download the template and import |

Change history uses the existing `audit:view`. Starting roles: Network engineer gets all eight; Security auditor gets `links:view` and `links:commercial_view`; IT officer, Requester and Viewer get `links:view`. Administrators have everything.

## Out of scope

Live monitoring, SNMP polling, VPN configuration, bandwidth use, discovery of links, failover, approval workflows, SLA calculations and incident tickets. Other tools do these.
