import {
  Bell, BookCheck, Boxes, Cable, FileUp, ClipboardList, FileBarChart, GitCompareArrows, LayoutDashboard, Network,
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
      { to: '/devices', label: 'Device Inventory', icon: Server },
      { to: '/interfaces', label: 'Interface Inventory', icon: Cable },
    ],
  },
  {
    group: 'Operations',
    items: [
      { to: '/discovery', label: 'Discovery Centre', icon: Radar },
      { to: '/imports', label: 'Imports', icon: FileUp },
      { to: '/reconciliation', label: 'Reconciliation Centre', icon: GitCompareArrows },
      { to: '/integrations', label: 'Integrations', icon: Plug, phase: 5, summary: 'Read-only connections to infrastructure platforms.',
        features: ['FortiGate, VMware, AWS, Azure, DNS and DHCP', 'Connection health and sync history'] },
    ],
  },
  {
    group: 'Governance',
    items: [
      { to: '/requests', label: 'IP Allocation Requests', icon: ClipboardList },
      { to: '/reservations', label: 'Reservations', icon: BookCheck },
      { to: '/alerts', label: 'Alerts and Notifications', icon: Bell },
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
