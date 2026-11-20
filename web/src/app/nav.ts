import {
  Activity, ArrowLeftRight, Bell, Import, MonitorPlay, BookCheck, Building2, Cable, CalendarClock, ClipboardList, FileBarChart, History, LayoutDashboard, List, MonitorSmartphone, Network,
  PlusCircle, Radar, Server, SlidersHorizontal, UsersRound, type LucideIcon,
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
  /** Active only on this exact address, not on the pages under it. */
  end?: boolean
}

const view = ['inventory:view']

export const nav: { group: string; items: NavItem[] }[] = [
  {
    group: 'Overview',
    items: [
      { to: '/', label: 'Dashboard', icon: LayoutDashboard, summary: 'Networks, alerts, devices and server health at a glance.' },
      { to: '/monitor', label: 'Monitor', icon: MonitorPlay, hidden: true, summary: 'Everything that needs attention, for a screen on the wall.' },
    ],
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
    group: 'Firewall',
    items: [
      { to: '/firewall/live', label: 'Live', icon: Activity, anyOf: ['inventory:view', 'links:view'], summary: 'What the FortiGate reports now: interfaces and traffic, VPN tunnels and users, SD-WAN health and load.' },
      { to: '/firewall/review', label: 'Review and import', icon: Import, anyOf: ['inventory:edit', 'network:manage', 'links:create', 'links:edit'], summary: 'Fetch networks, devices, servers and links from the FortiGate, check them, and add what you confirm.' },
    ],
  },
  {
    group: 'Network Links',
    items: [
      { to: '/network-links', label: 'Overview', icon: Cable, anyOf: ['links:view'], end: true, summary: 'Internet lines, VPN tunnels and leased lines at a glance: counts, costs and renewals.' },
      { to: '/network-links/all', label: 'All Network Links', icon: List, anyOf: ['links:view'], summary: 'Every connectivity link, with search, filters and export.' },
      { to: '/network-links/new', label: 'Add network link', icon: PlusCircle, anyOf: ['links:create'], hidden: true, summary: 'Register a new internet line, VPN tunnel or leased line.' },
      { to: '/network-links/providers', label: 'Service Providers', icon: Building2, anyOf: ['links:view'], summary: 'The companies that provide links, with their contacts.' },
      { to: '/network-links/renewals', label: 'Contract Renewals', icon: CalendarClock, anyOf: ['links:commercial_view'], summary: 'Link contracts ending soon, and those already ended.' },
      { to: '/network-links/import-export', label: 'Import / Export', icon: ArrowLeftRight, anyOf: ['links:import_export'], summary: 'Bring links in from Excel, and take them out.' },
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
      { to: '/system', label: 'System', icon: SlidersHorizontal, anyOf: ['system:manage'], summary: 'Intervals, email, sessions, FortiGate and Intune.' },
    ],
  },
]

export const allItems = nav.flatMap((g) => g.items.map((item) => ({ ...item, group: g.group })))

export const canSee = (item: Pick<NavItem, 'anyOf'>, permissions: string[]) => !item.anyOf || item.anyOf.some((p) => permissions.includes(p))

/** The menu for these permissions, without hidden pages or groups left empty. */
export const navFor = (permissions: string[]) =>
  nav.map((g) => ({ ...g, items: g.items.filter((i) => !i.hidden && canSee(i, permissions)) })).filter((g) => g.items.length > 0)

/** The page an address belongs to: the item whose address matches the most of it. */
export const pageFor = (pathname: string) =>
  allItems.filter((p) => (p.to === '/' ? pathname === '/' : pathname === p.to || pathname.startsWith(`${p.to}/`)))
    .sort((a, b) => b.to.length - a.to.length)[0]

/** Whether this address is open to someone holding these permissions. */
export function canOpen(pathname: string, permissions: string[]): boolean {
  const page = pageFor(pathname)
  return !page || canSee(page, permissions)
}
