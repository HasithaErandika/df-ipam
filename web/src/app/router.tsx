import { lazy } from 'react'
import { createBrowserRouter, Navigate } from 'react-router'
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
const Settings = page(() => import('../features/settings/SettingsPage'), 'SettingsPage')

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
      { path: 'settings/:tab?', element: <Settings /> },
      { path: '*', element: <Navigate to="/" replace /> },
    ],
  },
])
