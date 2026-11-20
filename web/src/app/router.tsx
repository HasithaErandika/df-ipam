import { lazy } from 'react'
import { createBrowserRouter, Navigate, useParams } from 'react-router'
import { AppShell } from './shell/AppShell'

// Each screen loads on first visit, so signing in doesn't download the whole app.
const page = <K extends string>(load: () => Promise<Record<K, React.ComponentType>>, name: K) =>
  lazy(() => load().then((m) => ({ default: m[name] })))

const Dashboard = page(() => import('../features/dashboard/DashboardPage'), 'DashboardPage')
const Discovery = page(() => import('../features/discovery/DiscoveryPage'), 'DiscoveryPage')
const SegmentPage = page(() => import('../features/discovery/SegmentPage'), 'SegmentPage')
const Servers = page(() => import('../features/servers/ServersPage'), 'ServersPage')
const Devices = page(() => import('../features/devices/DevicesPage'), 'DevicesPage')
const Alerts = page(() => import('../features/alerts/AlertsPage'), 'AlertsPage')
const Requests = page(() => import('../features/requests/RequestsPage'), 'RequestsPage')
const Reservations = page(() => import('../features/reservations/ReservationsPage'), 'ReservationsPage')
const Reports = page(() => import('../features/reports/ReportsPage'), 'ReportsPage')
const NetworkSetup = page(() => import('../features/network/NetworkSetupPage'), 'NetworkSetupPage')
const Access = page(() => import('../features/access/AccessPage'), 'AccessPage')
const Audit = page(() => import('../features/audit/AuditPage'), 'AuditPage')
const System = page(() => import('../features/system/SystemPage'), 'SystemPage')
const LinksOverview = page(() => import('../features/links/LinksOverviewPage'), 'LinksOverviewPage')
const LinksList = page(() => import('../features/links/LinksListPage'), 'LinksListPage')
const LinkDetail = page(() => import('../features/links/LinkDetailPage'), 'LinkDetailPage')
const LinkProviders = page(() => import('../features/links/ProvidersPage'), 'ProvidersPage')
const LinkRenewals = page(() => import('../features/links/RenewalsPage'), 'RenewalsPage')
const Firewall = page(() => import('../features/firewall/FirewallPage'), 'FirewallPage')
const Monitor = page(() => import('../features/monitor/MonitorPage'), 'MonitorPage')
const LinkTransfer = page(() => import('../features/links/ImportExportPage'), 'ImportExportPage')

// Settings was split into separate pages; old links land on the right one.
const moved: Record<string, string> = {
  settings: '/network-setup', environments: '/network-setup/environments', departments: '/network-setup/departments',
  users: '/access', audit: '/audit', system: '/system', integrations: '/system/fortigate',
}

function OldSettings() {
  const { tab } = useParams()
  return <Navigate to={moved[tab ?? 'settings'] ?? '/'} replace />
}

export const router = createBrowserRouter([
  {
    path: '/',
    element: <AppShell />,
    children: [
      { index: true, element: <Dashboard /> },
      { path: 'discovery/:tab?', element: <Discovery /> },
      { path: 'segments/:id', element: <SegmentPage /> },
      { path: 'servers/:tab?', element: <Servers /> },
      { path: 'devices/:tab?', element: <Devices /> },
      { path: 'alerts/:tab?', element: <Alerts /> },
      { path: 'requests/:tab?', element: <Requests /> },
      { path: 'reservations/:tab?', element: <Reservations /> },
      { path: 'reports', element: <Reports /> },
      { path: 'network-setup/:tab?', element: <NetworkSetup /> },
      { path: 'access/:tab?', element: <Access /> },
      { path: 'audit', element: <Audit /> },
      { path: 'system/:tab?', element: <System /> },
      { path: 'monitor', element: <Monitor /> },
      { path: 'network-links', element: <LinksOverview /> },
      { path: 'network-links/all', element: <LinksList /> },
      // Add and Edit open as dialogs over the list and the link's page.
      { path: 'network-links/new', element: <LinksList /> },
      { path: 'network-links/providers', element: <LinkProviders /> },
      { path: 'network-links/renewals', element: <LinkRenewals /> },
      { path: 'network-links/import-export', element: <LinkTransfer /> },
      { path: 'firewall/:tab?', element: <Firewall /> },
      { path: 'network-links/firewall/*', element: <Navigate to="/firewall/live" replace /> },
      { path: 'network-links/:id', element: <LinkDetail /> },
      { path: 'network-links/:id/edit', element: <LinkDetail /> },
      { path: 'settings/:tab?', element: <OldSettings /> },
      { path: '*', element: <Navigate to="/" replace /> },
    ],
  },
])
