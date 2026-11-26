# Network Links

**Network Links** is the record of the company's connectivity: internet leased lines, broadband, site-to-site and remote access VPNs, MPLS, leased lines, 4G/5G, AWS Direct Connect, Azure ExpressRoute and third-party connections. People keep it, and can bring links in from the FortiGate after reviewing them. A link's status says whether it is in service; whether it is up right now comes from the firewall's live view, never stored. **Nothing is configured:** the firewall is only read.

## Screens

The **Network Links** menu section. Adding a link isn't a menu item: **Add network link** on the Overview and the list opens the form in a dialog. The dialog has numbered tabs (Basic, Technical, VPN for VPN types, Related links, Commercial for people who manage costs, Notes and files) with Back and Next; a tab with a problem is marked red, and saving opens the first one.

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

The segment page lists the links on that subnet (its WAN subnet, one of the link's segments, or a WAN or gateway address inside it). A register entry's details list the links using that address (WAN IP, gateway, or inside the public range). Both show only to people with `links:view`.

## A link

The **Link ID** (`DF-LNK-0001`) is given when the link is saved and never changes. Fields marked required must be filled; everything else is optional, because not every link has every detail.

| Section | Fields |
| --- | --- |
| Basic (required: name, type, source location, purpose, role, criticality, status) | Name, link type, service provider, circuit / service ID, source location (a listed site; people with `network:manage` add a new one from the field, such as "BCP - Nugegoda"), destination (a site or an external party), business purpose, environment (Production, DR, UAT, Other), link role (Primary, Secondary, Backup, Standalone), criticality (Critical, High, Medium, Low), responsible officer (an active user), status (Active, Inactive, Pending, Terminated) |
| Technical | Download and upload bandwidth (Mbps or Gbps, stored in Mbps), connection medium (Fibre, Wireless, Copper, Other), WAN subnet (a segment, which may be a `/32` for a single address; chosen first), WAN IP (inside the WAN subnet: the register's addresses there are offered, or typed), more WAN addresses (each a subnet then an address, for a link with several, such as an ILL with two public `/32`s; up to 16), segments (every segment the link serves, as many as needed, for any link type: the LANs behind it, a VPN's local subnets), public IP or range (`203.0.113.0/29`), gateway IP, connected devices (as many approved devices as the link has, plus other devices' names as text; see below), interface or port, VLAN ID, VDOM / VRF, routing type (Static, BGP, OSPF, Other) |
| VPN, shown only for VPN link types | Tunnel name, VPN type (IPsec, SSL, Other), local and remote peer (IPv4 or a host name), remote subnets (the local subnets are the link's segments, on the Technical tab), remote organisation, remote technical contact, last connectivity test, credential vault reference |
| Commercial | No separate recurring cost (for a VPN paid for through its internet line, so it isn't counted twice), monthly rental and currency (LKR, USD or another code), annual recurring cost (monthly × 12, worked out), one-time installation cost, billing account, contract and purchase order references, activation, contract start and expiry dates, renewal notice period, cost centre |
| Support | Not entered on the link: the **service provider** gives the support, and the link's page shows the provider's hotline, support email, account manager and contacts. Support details a link already holds are kept and shown under "Recorded for this link" |
| Related links | Each related link and how: its **secondary** or **backup**, or the **underlying** link a VPN runs over. More than one backup is allowed. A link can't relate to itself, and a pair is stored once: if B already lists A, A can't list B |
| Remarks | Free text (the Notes and files tab) |
| Attachments | Files chosen in the dialog, each with its kind, uploaded to the link when it is saved. A file that fails is named, and can be added again from the link's page |

- **The WAN subnet can be added from the form.** When the WAN IP's network isn't a segment yet, people with `network:manage` get **Create segment** with that network: a small dialog (name, type, description) that defaults to the **Cloud** type, which is never swept. The new segment is audited like any other and becomes the link's WAN subnet. Others are told to ask; the address is kept on the link meanwhile.
- **Addresses are typed as the site diagrams write them.** The WAN IP and gateway take an address with its prefix or mask (`10.10.40.253 /24`, `122.255.4.226 255.255.255.248`) and keep the address; the form then picks the segment with that network as the WAN subnet, or says it isn't a segment yet. The public range takes a dotted mask too (`122.255.4.224 255.255.255.248` is `122.255.4.224/29`). Imports accept the same forms.
- **Connected devices from the scan.** The dialog asks which devices the scan saw in the last 30 days on the link's WAN addresses and gateway. On a new link, the approved ones are added to Connected devices until the person changes the list; on an edit they are offered with **+**. Devices not approved yet are named, to be approved in Devices first. Saving is the decision: the scan never links a device by itself. The link's page shows the connected devices and, separately, what the scan saw on its addresses.
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

Every three hours DF-Atlas checks contract expiry dates against the reminder days (90, 60, 30, 7 and 0, the expiry date, by default), in Sri Lanka time. When a link reaches one, its responsible officer and everyone with `links:commercial_manage` get a notification, and an email when email is on and their preference allows (kind "Network link contracts about to expire"). Each reminder goes once per contract end: days already passed are marked together, so a link entered 20 days before expiry gets one notice, not three. A new expiry date starts the reminders again and is recorded as a **contract renewal**. Terminated and archived links get none, nor do contracts that ended more than 7 days ago.

## Import and export

**Export** gives the links matching the list's filters (or all links) as Excel, CSV or text, up to 20,000 links (more is refused: narrow the filters). Every export is recorded in the audit log. Excel adds a **Subnets** sheet (one row per segment or remote subnet) and a **Related links** sheet.

**Import** reads Excel or CSV, up to 2,000 rows and 5 MB, using the **template**'s headers. Its second sheet lists the accepted values. A name that belongs to more than one device or other record is reported for the row, so give the MAC or other unique value. A row may relate to a link added earlier in the same file. Link types may be given by name or key, providers by name, officers by email, subnets and segments by network or name (the **Segments** column; older files' **Local Subnets** is read the same way), connected devices by name or MAC separated by `;` (a name that isn't a device is kept as text; older files' **Connected Device** is read too), more WAN IPs as `122.255.35.1/32; 10.0.0.9/24`, related links as `DF-LNK-0002:backup; DF-LNK-0003:underlying`.

1. **Check the file.** A dry run does everything and keeps nothing, listing each row to add or update and each row skipped, with why.
2. **Import.** Rows with problems are skipped; the rest go in, each audited.

A row whose **Link ID** exists, or whose provider and circuit ID match a live link, **updates** that link, but only when **Update existing links** is ticked; otherwise it is skipped as a duplicate. Columns the file leaves out keep their values. An unknown Link ID is refused. New links get the next Link ID.

## Reports

**Reports** has three more, for people who also hold the links permissions: **Network links** (every live link, `links:view`), **Network link costs** (monthly and annual cost per link in its own currency, with billing and contract details, also `links:commercial_view`) and **Network link contract renewals** (contracts ending in a date range that looks ahead, 90 days by default, also `links:commercial_view`). Archived links are left out, and terminated ones from costs and renewals. A report someone may not use isn't listed, and asking for it directly is refused.

## History

Every change is in the audit log in the same transaction: added, changed (each field before and after, related links and segments included), contract renewed, archived, restored, file attached and file removed, with who and when. The link's **Change history** shows them field by field to people with `audit:view`, leaving commercial fields out for those who may not see them. The general **Audit log** does the same: without `links:commercial_view`, a link's commercial fields and contract or quotation file names are removed from its records and exports, and its values aren't searched.

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

`links:commercial_manage` always comes with `links:commercial_view`: a role saved with manage gets view too, since the form can't change costs it can't show. Change history uses the existing `audit:view`. Starting roles: Network engineer gets all eight; Security auditor gets `links:view` and `links:commercial_view`; IT officer, Requester and Viewer get `links:view`. Administrators have everything.

## The firewall

WAN lines and IPsec tunnels can be brought in from the FortiGate after review, and matching links show live state. See [firewall.md](firewall.md).

## Out of scope

SNMP polling, any change to the firewall, failover, approval workflows, SLA calculations and incident tickets. Live firewall data is shown, not kept as history.
