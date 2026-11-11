import {
  Bell, BookCheck, ClipboardList, FileBarChart, LayoutDashboard, MonitorSmartphone, Server, Settings, type LucideIcon,
} from 'lucide-react'

export interface NavItem {
  to: string
  label: string
  icon: LucideIcon
  /** Who may open the page: holding any one is enough. Keep this in step with the routes the page reads, as the API does the real check. */
  anyOf?: string[]
  /** Build step in docs/new.md, for pages that aren't built yet. */
  step?: number
  summary?: string
  features?: string[]
}

const view = ['inventory:view']

export const nav: { group: string; items: NavItem[] }[] = [
  {
    group: 'Overview',
    items: [{
      to: '/', label: 'Dashboard', icon: LayoutDashboard, step: 7,
      summary: 'Network inventory and monitoring at a glance.',
      features: ['Floor DHCP pool usage', 'Open alerts by severity', 'Servers down per environment'],
    }],
  },
  {
    group: 'Network',
    items: [
      {
        to: '/servers', label: 'Servers', icon: Server, anyOf: view, step: 2,
        summary: 'The server register, by environment, with live status.',
        features: ['Tabs for All, Production, UAT, Development, DR and Test', 'Up, Down or Unknown from checks every minute', 'Add a server with a suggested Available candidate IP', 'Critical alert when a Production server goes down or its MAC changes'],
      },
      {
        to: '/devices', label: 'Devices', icon: MonitorSmartphone, anyOf: view, step: 2,
        summary: 'Known MAC addresses, and new ones waiting for a decision.',
        features: ['Approved, pending and rejected lists', 'Approve or reject a new device, and register its IP', 'Import an existing device list from CSV'],
      },
      {
        to: '/alerts', label: 'Alerts', icon: Bell, anyOf: view, step: 4,
        summary: 'Unknown devices, unregistered static IPs, conflicts and servers down.',
        features: ['Open and recent alerts by severity', 'Acknowledge an alert', 'Alerts close by themselves when the cause is gone', 'Email for High and Critical'],
      },
    ],
  },
  {
    group: 'Requests',
    items: [
      {
        to: '/requests', label: 'IP Requests', icon: ClipboardList, anyOf: ['ip:request', 'requests:approve'], step: 5,
        summary: 'Ask for an IP, and approve requests in one step.',
        features: ['Raise a request for a server, printer or other device', 'Approval queue for Admins, with a suggested Available candidate IP', 'The requester is told the IP once approved'],
      },
      {
        to: '/reservations', label: 'Reservations', icon: BookCheck, anyOf: view, step: 5,
        summary: 'Hold an IP or a small range for a purpose.',
        features: ['Reason, owner and an optional expiry date', 'Reminder 7 days before expiry', 'Extend, turn into an assignment, or release. Nothing is released automatically'],
      },
    ],
  },
  {
    group: 'Insight',
    items: [{
      to: '/reports', label: 'Reports', icon: FileBarChart, anyOf: ['reports:export'], step: 6,
      summary: 'CSV and PDF reports, made when you ask.',
      features: ['Segment usage and the IP register', 'Servers by environment and their availability', 'Unknown devices and static IPs on floors', 'Alert history, requests, reservations and the audit log'],
    }],
  },
  {
    group: 'Administration',
    items: [{
      to: '/settings', label: 'Settings', icon: Settings, anyOf: ['system:manage', 'users:manage', 'audit:view'], step: 2,
      summary: 'Segments, environments, users and the audit log.',
      features: ['Floor and server segments with VLAN, network and DHCP pool', 'Environments such as Production and UAT', 'Users and their role: Admin, Requester or Viewer', 'Audit log of every change'],
    }],
  },
]

export const allItems = nav.flatMap((g) => g.items.map((item) => ({ ...item, group: g.group })))

export const canSee = (item: Pick<NavItem, 'anyOf'>, permissions: string[]) => !item.anyOf || item.anyOf.some((p) => permissions.includes(p))

/** The menu for these permissions, without groups left empty. */
export const navFor = (permissions: string[]) =>
  nav.map((g) => ({ ...g, items: g.items.filter((i) => canSee(i, permissions)) })).filter((g) => g.items.length > 0)

/** Whether this address is open to someone holding these permissions. */
export function canOpen(pathname: string, permissions: string[]): boolean {
  const page = allItems.find((p) => (p.to === '/' ? pathname === '/' : pathname === p.to || pathname.startsWith(`${p.to}/`)))
  return !page || canSee(page, permissions)
}
