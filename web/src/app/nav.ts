import {
  Bell, BookCheck, Boxes, Cable, FileUp, ClipboardList, FileBarChart, GitCompareArrows, LayoutDashboard, Network,
  Plug, Radar, ScrollText, Server, Settings, ShieldCheck, TableProperties, Users, type LucideIcon,
} from 'lucide-react'

export interface NavItem {
  to: string
  label: string
  icon: LucideIcon
  /** Who may open the page: holding any one is enough. Keep this in step with the routes the page reads, as the API does the real check. */
  anyOf?: string[]
  /** Roadmap phase for pages that aren't built yet. */
  phase?: number
  summary?: string
  features?: string[]
}

const view = ['inventory:view']

export const nav: { group: string; items: NavItem[] }[] = [
  { group: 'Overview', items: [{ to: '/', label: 'Dashboard', icon: LayoutDashboard }] },
  {
    group: 'Inventory',
    items: [
      { to: '/network', label: 'Network Management', icon: Network, anyOf: view },
      { to: '/subnets', label: 'Subnet Management', icon: TableProperties, anyOf: view },
      { to: '/ip-addresses', label: 'IP Address Explorer', icon: Boxes, anyOf: ['inventory:view', 'inventory:view_scoped'] },
      { to: '/devices', label: 'Device Inventory', icon: Server, anyOf: view },
      { to: '/interfaces', label: 'Interface Inventory', icon: Cable, anyOf: view },
    ],
  },
  {
    group: 'Operations',
    items: [
      { to: '/discovery', label: 'Discovery Centre', icon: Radar, anyOf: ['discovery:run', 'discovery:manage', 'audit:view'] },
      { to: '/imports', label: 'Imports', icon: FileUp, anyOf: ['imports:run', 'audit:view'] },
      { to: '/reconciliation', label: 'Reconciliation Centre', icon: GitCompareArrows, anyOf: view },
      { to: '/integrations', label: 'Integrations', icon: Plug, anyOf: ['integrations:manage', 'audit:view'] },
    ],
  },
  {
    group: 'Governance',
    items: [
      { to: '/requests', label: 'IP Allocation Requests', icon: ClipboardList, anyOf: ['ip:request', 'requests:approve', 'ip:allocate', 'ip:reserve', 'audit:view'] },
      { to: '/reservations', label: 'Reservations', icon: BookCheck, anyOf: view },
      { to: '/alerts', label: 'Alerts and Notifications', icon: Bell },
      { to: '/reports', label: 'Reports', icon: FileBarChart, anyOf: ['reports:export', 'reports:export_scoped'] },
      { to: '/audit', label: 'Audit Logs', icon: ScrollText, anyOf: ['audit:view'] },
    ],
  },
  {
    group: 'Administration',
    items: [
      { to: '/users', label: 'User Management', icon: Users, anyOf: ['users:manage'] },
      { to: '/roles', label: 'Roles and Permissions', icon: ShieldCheck, anyOf: ['users:manage', 'audit:view'] },
      { to: '/system', label: 'System Administration', icon: Settings, anyOf: ['system:manage', 'audit:view'] },
    ],
  },
]

export const allItems = nav.flatMap((g) => g.items.map((item) => ({ ...item, group: g.group })))

/** Pages with no menu link still have a rule. */
const pages: Pick<NavItem, 'to' | 'anyOf'>[] = [...allItems, { to: '/search', anyOf: view }]

export const canSee = (item: Pick<NavItem, 'anyOf'>, permissions: string[]) => !item.anyOf || item.anyOf.some((p) => permissions.includes(p))

/** The menu for these permissions, without groups left empty. */
export const navFor = (permissions: string[]) =>
  nav.map((g) => ({ ...g, items: g.items.filter((i) => canSee(i, permissions)) })).filter((g) => g.items.length > 0)

/** Whether this address is open to someone holding these permissions. */
export function canOpen(pathname: string, permissions: string[]): boolean {
  const page = pages.find((p) => (p.to === '/' ? pathname === '/' : pathname === p.to || pathname.startsWith(`${p.to}/`)))
  return !page || canSee(page, permissions)
}
