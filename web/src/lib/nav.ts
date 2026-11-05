import {
  Bell, BookCheck, Boxes, Cable, ClipboardList, FileBarChart, GitCompareArrows, LayoutDashboard, Network,
  Plug, Radar, ScrollText, Server, Settings, ShieldCheck, TableProperties, Users, type LucideIcon,
} from 'lucide-react'

export interface NavItem {
  to: string
  label: string
  icon: LucideIcon
  /** Roadmap phase for pages that aren't built yet. */
  phase?: number
  summary?: string
  features?: string[]
}

export const nav: { group: string; items: NavItem[] }[] = [
  { group: 'Overview', items: [{ to: '/', label: 'Dashboard', icon: LayoutDashboard }] },
  {
    group: 'Inventory',
    items: [
      { to: '/network', label: 'Network Management', icon: Network },
      { to: '/subnets', label: 'Subnet Management', icon: TableProperties },
      { to: '/ip-addresses', label: 'IP Address Explorer', icon: Boxes },
      { to: '/devices', label: 'Device Inventory', icon: Server, phase: 3, summary: 'One record per device, however many sources report it.',
        features: ['Firewalls, routers, switches, servers and VMs', 'Interfaces, MACs, IPs and services per device', 'Discovery history and source tracking'] },
      { to: '/interfaces', label: 'Interface Inventory', icon: Cable, phase: 3, summary: 'Search interfaces across all devices.',
        features: ['Admin and operational status', 'VLAN, VDOM and VRF context', 'Linked IP addresses'] },
    ],
  },
  {
    group: 'Operations',
    items: [
      { to: '/discovery', label: 'Discovery Centre', icon: Radar, phase: 4, summary: 'Controlled discovery within approved scopes.',
        features: ['Agents, policies, schedules and job history', 'ICMP, ARP/NDP, DNS, SNMP and narrow Nmap', 'Rate limits, windows and exclusions', 'Emergency stop'] },
      { to: '/reconciliation', label: 'Reconciliation Centre', icon: GitCompareArrows },
      { to: '/integrations', label: 'Integrations', icon: Plug, phase: 5, summary: 'Read-only connections to infrastructure platforms.',
        features: ['FortiGate, VMware, AWS, Azure, DNS and DHCP', 'Connection health and sync history'] },
    ],
  },
  {
    group: 'Governance',
    items: [
      { to: '/requests', label: 'IP Allocation Requests', icon: ClipboardList, phase: 2, summary: 'Request, approve and track IP allocations.',
        features: ['Multi-level approval workflows', 'Separation of duties', 'Concurrency-safe allocation'] },
      { to: '/reservations', label: 'Reservations', icon: BookCheck, phase: 2, summary: 'Hold addresses for planned work.',
        features: ['Expiry and reminders', 'Reservation history'] },
      { to: '/alerts', label: 'Alerts and Notifications', icon: Bell, phase: 2, summary: 'What needs attention, without the noise.',
        features: ['Conflicts, utilisation and discovery failures', 'Per-event rules and recipient groups', 'Alert deduplication'] },
      { to: '/reports', label: 'Reports', icon: FileBarChart, phase: 6, summary: 'Registers and evidence in Excel, CSV and PDF.',
        features: ['Allocation, subnet and device registers', 'Utilisation trends and ownership gaps', 'Large exports run in the background'] },
      { to: '/audit', label: 'Audit Logs', icon: ScrollText },
    ],
  },
  {
    group: 'Administration',
    items: [
      { to: '/users', label: 'User Management', icon: Users },
      { to: '/roles', label: 'Roles and Permissions', icon: ShieldCheck, phase: 7, summary: 'Review and adjust the permission matrix.',
        features: ['Six default roles', 'Granular permissions and scopes'] },
      { to: '/system', label: 'System Administration', icon: Settings, phase: 7, summary: 'Platform settings and health.',
        features: ['Security policies and retention', 'Imports and enrichment queue', 'Notifications and backup settings'] },
    ],
  },
]

export const allItems = nav.flatMap((g) => g.items.map((item) => ({ ...item, group: g.group })))
