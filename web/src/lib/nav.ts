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
      { to: '/integrations', label: 'Integrations', icon: Plug },
    ],
  },
  {
    group: 'Governance',
    items: [
      { to: '/requests', label: 'IP Allocation Requests', icon: ClipboardList },
      { to: '/reservations', label: 'Reservations', icon: BookCheck },
      { to: '/alerts', label: 'Alerts and Notifications', icon: Bell },
      { to: '/reports', label: 'Reports', icon: FileBarChart },
      { to: '/audit', label: 'Audit Logs', icon: ScrollText },
    ],
  },
  {
    group: 'Administration',
    items: [
      { to: '/users', label: 'User Management', icon: Users },
      { to: '/roles', label: 'Roles and Permissions', icon: ShieldCheck },
      { to: '/system', label: 'System Administration', icon: Settings },
    ],
  },
]

export const allItems = nav.flatMap((g) => g.items.map((item) => ({ ...item, group: g.group })))
