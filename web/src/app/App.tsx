import { useQueryClient } from '@tanstack/react-query'
import { ShieldAlert } from 'lucide-react'
import { lazy, Suspense } from 'react'
import { RouterProvider } from 'react-router'
import { Button, ErrorState } from '../components/ui'
import { signOut } from '../lib/auth'
import { router } from './router'
import { MeProvider, useMeQuery, useSignedIn } from '../lib/session'

const SignIn = lazy(() => import('../features/auth/SignIn').then((module) => ({ default: module.SignIn })))

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
  return signedIn ? <Authenticated /> : <Suspense fallback={null}><SignIn /></Suspense>
}
