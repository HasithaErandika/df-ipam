import { useQueryClient } from '@tanstack/react-query'
import { ChevronRight } from 'lucide-react'
import { Suspense, useEffect, useState } from 'react'
import { Link, Outlet, useLocation } from 'react-router'
import { Button, Card, EmptyState, ErrorBoundary, Modal, SkeletonRows } from '../../components/ui'
import { signOut } from '../../lib/auth'
import { useIdleTimeout } from '../../lib/idle'
import { useCan, useMe } from '../../lib/session'
import { useScanStatus } from '../../lib/scan'
import { canOpen, pageFor } from '../nav'
import { NotificationBell } from './NotificationBell'
import { Sidebar } from './Sidebar'
import { UserMenu } from './TopBar'

/** A link or an old bookmark can still reach a page this role can't use. */
function Page({ pathname }: { pathname: string }) {
  const { permissions } = useMe()
  if (canOpen(pathname, permissions)) return <Outlet />
  return <Card><EmptyState title="You don't have access to this page" body="Your role doesn't include it. Ask an administrator if you need it." action={<Link className="text-accent hover:underline" to="/">Go to the dashboard</Link>} /></Card>
}

function readCollapsed(): boolean {
  try {
    return localStorage.getItem('dfipam.sidebar') === 'collapsed' || window.innerWidth < 1024
  } catch {
    return window.innerWidth < 1024
  }
}

/** Keeps screens current: reloads results whenever a scan finishes. */
function ScanWatcher() {
  useScanStatus()
  return null
}

/** Signs out after the idle time set in Settings, with a minute's warning. */
function SessionGuard() {
  const { sessionTimeoutMinutes } = useMe()
  const queryClient = useQueryClient()
  const leave = async () => { await signOut(); queryClient.clear() }
  const { secondsLeft, stay } = useIdleTimeout(sessionTimeoutMinutes || 30, () => void leave())
  return (
    <Modal open={secondsLeft !== null} onClose={stay} title="Are you still there?"
      footer={<><Button variant="ghost" onClick={() => void leave()}>Sign out now</Button><Button variant="primary" onClick={stay}>Stay signed in</Button></>}>
      <p className="text-ink-2">For security, you'll be signed out in <strong className="text-ink tabular-nums">{secondsLeft}</strong> seconds because there has been no activity.</p>
    </Modal>
  )
}

export function AppShell() {
  const { pathname } = useLocation()
  const canView = useCan('inventory:view')
  const [collapsed, setCollapsed] = useState(readCollapsed)
  const toggle = () => setCollapsed((c) => {
    try {
      localStorage.setItem('dfipam.sidebar', c ? 'open' : 'collapsed')
    } catch {
      // The choice simply isn't remembered.
    }
    return !c
  })
  const current = pageFor(pathname)
  useEffect(() => { document.title = current ? `${current.label} · DF-IPAM` : 'DF-IPAM' }, [current])
  return (
    <div className="flex h-full">
      <Sidebar collapsed={collapsed} onToggle={toggle} />
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-[60px] shrink-0 items-center justify-between gap-4 border-b border-line bg-surface px-6 max-sm:px-4">
          <nav aria-label="Breadcrumb" className="flex min-w-0 items-center gap-1 text-[12.5px] text-ink-3">
            <span className="truncate max-md:hidden">{current?.group}</span>
            <ChevronRight size={13} className="shrink-0 max-md:hidden" aria-hidden />
            <span aria-current="page" className="truncate font-medium text-ink">{current?.label}</span>
          </nav>
          <div className="flex items-center gap-1.5">
            <NotificationBell />
            <UserMenu />
          </div>
        </header>
        <main className="min-h-0 flex-1 overflow-y-auto">
          <div className="mx-auto max-w-[1440px] px-6 py-5 max-sm:px-4">
            <ErrorBoundary resetKey={pathname}><Suspense fallback={<SkeletonRows rows={8} />}><Page pathname={pathname} /></Suspense></ErrorBoundary>
          </div>
        </main>
      </div>
      <SessionGuard />
      {canView && <ScanWatcher />}
    </div>
  )
}
