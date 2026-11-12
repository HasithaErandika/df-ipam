import {
  Bell, BookCheck, ClipboardList, FileBarChart, History, LayoutDashboard, MonitorSmartphone, Network, Radar, Server, SlidersHorizontal, UsersRound, type LucideIcon,
} from 'lucide-react'

export interface NavItem {
  to: string
  label: string
  icon: LucideIcon
  /** Who may open the page: holding any one is enough. The API does the real check. */
  anyOf?: string[]
  summary: string
  /** Pages reached from another page, not from the menu. */
  hidden?: boolean
  /** A count shown beside the item, from the sidebar's badge source of this name. */
  badge?: 'alerts' | 'devices' | 'requests'
}

const view = ['inventory:view']

export const nav: { group: string; items: NavItem[] }[] = [
  {
    group: 'Overview',
    items: [{ to: '/', label: 'Dashboard', icon: LayoutDashboard, summary: 'Floors, alerts and server health at a glance.' }],
  },
  {
    group: 'Network',
    items: [
      { to: '/discovery', label: 'Discovery', icon: Radar, anyOf: view, summary: 'What the scanner has seen on the network, and when it last ran.' },
      { to: '/servers', label: 'Servers', icon: Server, anyOf: view, summary: 'The server register by environment, with live status, and the full IP register.' },
      { to: '/devices', label: 'Devices', icon: MonitorSmartphone, anyOf: view, summary: 'Known MAC addresses, and new ones waiting for a decision.', badge: 'devices' },
      { to: '/alerts', label: 'Alerts', icon: Bell, anyOf: view, summary: 'Unknown devices, unregistered IPs, conflicts and servers down.', badge: 'alerts' },
      { to: '/segments', label: 'Segment', icon: Radar, anyOf: view, summary: 'Every registered and seen address in one segment.', hidden: true },
    ],
  },
  {
    group: 'Requests',
    items: [
      { to: '/requests', label: 'IP Requests', icon: ClipboardList, anyOf: ['ip:request', 'requests:approve'], summary: 'Ask for an IP, and approve requests in one step.', badge: 'requests' },
      { to: '/reservations', label: 'Reservations', icon: BookCheck, anyOf: view, summary: 'Hold an IP or a small range for a purpose. Nothing is released automatically.' },
    ],
  },
  {
    group: 'Insight',
    items: [
      { to: '/reports', label: 'Reports', icon: FileBarChart, anyOf: ['reports:export'], summary: 'CSV, Excel and PDF reports, made when you ask.' },
      { to: '/audit', label: 'Audit log', icon: History, anyOf: ['audit:view'], summary: 'Every change, who made it and when.' },
    ],
  },
  {
    group: 'Administration',
    items: [
      { to: '/network-setup', label: 'Network setup', icon: Network, anyOf: ['network:manage'], summary: 'Segments, environments and departments.' },
      { to: '/access', label: 'Users and roles', icon: UsersRound, anyOf: ['users:manage'], summary: 'Who may sign in, and what each role may do.' },
      { to: '/system', label: 'System', icon: SlidersHorizontal, anyOf: ['system:manage'], summary: 'Intervals, email, sessions and the FortiGate connection.' },
    ],
  },
]

export const allItems = nav.flatMap((g) => g.items.map((item) => ({ ...item, group: g.group })))

export const canSee = (item: Pick<NavItem, 'anyOf'>, permissions: string[]) => !item.anyOf || item.anyOf.some((p) => permissions.includes(p))

/** The menu for these permissions, without hidden pages or groups left empty. */
export const navFor = (permissions: string[]) =>
  nav.map((g) => ({ ...g, items: g.items.filter((i) => !i.hidden && canSee(i, permissions)) })).filter((g) => g.items.length > 0)

export const pageFor = (pathname: string) =>
  allItems.find((p) => (p.to === '/' ? pathname === '/' : pathname === p.to || pathname.startsWith(`${p.to}/`)))

/** Whether this address is open to someone holding these permissions. */
export function canOpen(pathname: string, permissions: string[]): boolean {
  const page = pageFor(pathname)
  return !page || canSee(page, permissions)
}
