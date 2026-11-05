import { useQueryClient } from '@tanstack/react-query'
import { ShieldAlert } from 'lucide-react'
import { createBrowserRouter, Navigate, RouterProvider } from 'react-router'
import { Button, ErrorState } from './components/ui'
import { AppShell } from './layouts/AppShell'
import { signOut } from './lib/auth'
import { allItems } from './lib/nav'
import { MeProvider, useMeQuery, useSignedIn } from './lib/session'
import { Alerts } from './pages/Alerts'
import { Audit } from './pages/Audit'
import { Dashboard } from './pages/Dashboard'
import { Devices, Interfaces } from './pages/Devices'
import { Discovery } from './pages/Discovery'
import { Imports } from './pages/Imports'
import { Integrations } from './pages/Integrations'
import { Explorer } from './pages/Explorer'
import { Network } from './pages/Network'
import { Planned } from './pages/Planned'
import { Reconciliation } from './pages/Reconciliation'
import { Reports } from './pages/Reports'
import { Requests } from './pages/Requests'
import { Roles } from './pages/Roles'
import { SearchResults } from './pages/SearchResults'
import { Reservations } from './pages/Reservations'
import { SignIn } from './pages/SignIn'
import { Subnets } from './pages/Subnets'
import { System } from './pages/System'
import { Users } from './pages/Users'

const router = createBrowserRouter([
  {
    path: '/',
    element: <AppShell />,
    children: [
      { index: true, element: <Dashboard /> },
      { path: 'network', element: <Network /> },
      { path: 'subnets', element: <Subnets /> },
      { path: 'audit', element: <Audit /> },
      { path: 'reconciliation', element: <Reconciliation /> },
      { path: 'users', element: <Users /> },
      { path: 'requests', element: <Requests /> },
      { path: 'reservations', element: <Reservations /> },
      { path: 'alerts', element: <Alerts /> },
      { path: 'ip-addresses', element: <Explorer /> },
      { path: 'search', element: <SearchResults /> },
      { path: 'devices', element: <Devices /> },
      { path: 'interfaces', element: <Interfaces /> },
      { path: 'discovery', element: <Discovery /> },
      { path: 'imports', element: <Imports /> },
      { path: 'integrations', element: <Integrations /> },
      { path: 'reports', element: <Reports /> },
      { path: 'roles', element: <Roles /> },
      { path: 'system', element: <System /> },
      ...allItems.filter((i) => i.phase).map((item) => ({ path: item.to.slice(1), element: <Planned item={item} /> })),
      { path: '*', element: <Navigate to="/" replace /> },
    ],
  },
])

function Centred({ children }: { children: React.ReactNode }) {
  return <div className="grid h-full place-items-center bg-bg p-6"><div className="w-full max-w-md rounded-lg border border-line bg-surface">{children}</div></div>
}

function Authenticated() {
  const queryClient = useQueryClient()
  const { data: me, isPending, error, refetch } = useMeQuery()
  const leave = async () => { await signOut(); queryClient.clear() }
  if (isPending) return <div role="status" aria-label="Loading" className="h-full bg-bg" />
  if (error) {
    return (
      <Centred>
        <ErrorState error={error} onRetry={refetch} />
        <div className="pb-5 text-center"><Button variant="ghost" onClick={leave}>Sign out</Button></div>
      </Centred>
    )
  }
  if (me.roles.length === 0) {
    return (
      <Centred>
        <div className="flex flex-col items-center p-8 text-center">
          <ShieldAlert size={28} className="mb-3 text-ink-3" aria-hidden />
          <h1 className="text-base font-semibold text-ink">Waiting for access</h1>
          <p className="mt-1 text-ink-2">{me.email} is signed in but has no role yet. Ask your IPAM administrator to assign one.</p>
          <div className="mt-5 flex gap-2"><Button onClick={() => refetch()}>Check again</Button><Button variant="ghost" onClick={leave}>Sign out</Button></div>
        </div>
      </Centred>
    )
  }
  return <MeProvider me={me}><RouterProvider router={router} /></MeProvider>
}

export function App() {
  const signedIn = useSignedIn()
  if (signedIn === null) return null
  return signedIn ? <Authenticated /> : <SignIn />
}
