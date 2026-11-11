import { createBrowserRouter, Navigate } from 'react-router'
import { Planned } from '../features/planned/Planned'
import { InventoryPage } from '../features/inventory/InventoryPage'
import { DashboardPage } from '../features/dashboard/DashboardPage'
import { AlertsPage } from '../features/alerts/AlertsPage'
import { RequestsPage } from '../features/requests/RequestsPage'
import { ReservationsPage } from '../features/reservations/ReservationsPage'
import { allItems } from './nav'
import { AppShell } from './shell/AppShell'

/** Every Lite screen shows what it will do until its build step lands (docs/new.md). */
export const router = createBrowserRouter([
  {
    path: '/',
    element: <AppShell />,
    children: [
      ...allItems.map((item) => (item.to === '/'
        ? { index: true, element: <DashboardPage /> }
        : { path: `${item.to.slice(1)}/:tab?`, element: item.to === '/alerts' ? <AlertsPage /> : item.to === '/requests' ? <RequestsPage /> : item.to === '/reservations' ? <ReservationsPage /> : ['/servers','/devices','/settings'].includes(item.to) ? <InventoryPage item={item} /> : <Planned item={item} /> })),
      { path: '*', element: <Navigate to="/" replace /> },
    ],
  },
])
