import { createBrowserRouter, Navigate } from 'react-router'
import { Planned } from '../features/planned/Planned'
import { InventoryPage } from '../features/inventory/InventoryPage'
import { allItems } from './nav'
import { AppShell } from './shell/AppShell'

/** Every Lite screen shows what it will do until its build step lands (docs/new.md). */
export const router = createBrowserRouter([
  {
    path: '/',
    element: <AppShell />,
    children: [
      ...allItems.map((item) => (item.to === '/'
        ? { index: true, element: <Planned item={item} /> }
        : { path: `${item.to.slice(1)}/:tab?`, element: ['/floors','/servers','/devices','/settings'].includes(item.to) ? <InventoryPage item={item} /> : <Planned item={item} /> })),
      { path: '*', element: <Navigate to="/" replace /> },
    ],
  },
])
