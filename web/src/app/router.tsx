import { type ComponentType, lazy } from 'react'
import { createBrowserRouter, Navigate } from 'react-router'
import { Planned } from '../features/planned/Planned'
import { allItems } from './nav'
import { AppShell } from './shell/AppShell'

/** Loads a screen the first time it is opened, so sign-in doesn't wait for all of them. */
function screen<K extends string>(load: () => Promise<Record<K, ComponentType>>, name: K) {
  const Page: ComponentType = lazy(() => load().then((module): { default: ComponentType } => ({ default: module[name] })))
  return <Page />
}

export const router = createBrowserRouter([
  {
    path: '/',
    element: <AppShell />,
    children: [
      { index: true, element: screen(() => import('../features/dashboard/Dashboard'), 'Dashboard') },
      { path: 'network/:tab?', element: screen(() => import('../features/network/Network'), 'Network') },
      { path: 'subnets', element: screen(() => import('../features/subnets/Subnets'), 'Subnets') },
      { path: 'audit', element: screen(() => import('../features/audit/Audit'), 'Audit') },
      { path: 'reconciliation/:tab?', element: screen(() => import('../features/reconciliation/Reconciliation'), 'Reconciliation') },
      { path: 'users', element: screen(() => import('../features/users/Users'), 'Users') },
      { path: 'requests/:tab?', element: screen(() => import('../features/requests/Requests'), 'Requests') },
      { path: 'reservations', element: screen(() => import('../features/reservations/Reservations'), 'Reservations') },
      { path: 'alerts/:tab?', element: screen(() => import('../features/alerts/Alerts'), 'Alerts') },
      { path: 'ip-addresses/:tab?', element: screen(() => import('../features/explorer/Explorer'), 'Explorer') },
      { path: 'search', element: screen(() => import('../features/search/SearchResults'), 'SearchResults') },
      { path: 'devices/:tab?', element: screen(() => import('../features/devices/Devices'), 'Devices') },
      { path: 'interfaces', element: screen(() => import('../features/devices/Devices'), 'Interfaces') },
      { path: 'discovery/:tab?', element: screen(() => import('../features/discovery/Discovery'), 'Discovery') },
      { path: 'imports/:tab?', element: screen(() => import('../features/imports/Imports'), 'Imports') },
      { path: 'integrations', element: screen(() => import('../features/integrations/Integrations'), 'Integrations') },
      { path: 'reports', element: screen(() => import('../features/reports/Reports'), 'Reports') },
      { path: 'roles', element: screen(() => import('../features/roles/Roles'), 'Roles') },
      { path: 'system', element: screen(() => import('../features/system/System'), 'System') },
      ...allItems.filter((i) => i.phase).map((item) => ({ path: item.to.slice(1), element: <Planned item={item} /> })),
      { path: '*', element: <Navigate to="/" replace /> },
    ],
  },
])
