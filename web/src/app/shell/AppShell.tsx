import { ChevronRight } from 'lucide-react'
import { Suspense, useEffect, useState } from 'react'
import { Link, Outlet, useLocation } from 'react-router'
import { Sidebar } from './Sidebar'
import { UserMenu } from './TopBar'
import { Card, EmptyState, ErrorBoundary, SkeletonRows } from '../../components/ui'
import { useMe } from '../../lib/session'
import { allItems, canOpen } from '../nav'

/** A link or an old bookmark can still reach a page this role can't use. */
function Page({ pathname }: { pathname: string }) {
  const { permissions } = useMe()
  if (canOpen(pathname, permissions)) return <Outlet />
  return <Card><EmptyState title="You don't have access to this page" body="Your role doesn't include it. Ask an administrator if you need it." action={<Link className="text-accent hover:underline" to="/">Go to the dashboard</Link>} /></Card>
}

export function AppShell() {
  const { pathname } = useLocation()
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem('dfipam.sidebar') === 'collapsed' || window.innerWidth < 1024)
  const toggle = () => setCollapsed((c) => {
    localStorage.setItem('dfipam.sidebar', c ? 'open' : 'collapsed')
    return !c
  })
  const current = allItems.find((item) => (item.to === '/' ? pathname === '/' : pathname.startsWith(item.to)))
  useEffect(() => { document.title = current ? `${current.label} · DF-IPAM` : 'DF-IPAM' }, [current])
  return (
    <div className="flex h-full">
      <Sidebar collapsed={collapsed} onToggle={toggle} />
      <div className="flex min-w-0 flex-1 flex-col">
        {/* Search and the notification bell return with their build steps (docs/new.md). */}
        <header className="flex h-[60px] shrink-0 items-center justify-between gap-4 border-b border-line bg-surface px-6">
          <nav aria-label="Breadcrumb" className="flex min-w-0 items-center gap-1 text-[12.5px] text-ink-3">
            <span className="truncate max-md:hidden">{current?.group}</span>
            <ChevronRight size={13} className="shrink-0 max-md:hidden" aria-hidden />
            <span aria-current="page" className="truncate font-medium text-ink">{current?.label}</span>
          </nav>
          <UserMenu />
        </header>
        <main className="min-h-0 flex-1 overflow-y-auto">
          <div className="mx-auto max-w-[1440px] px-6 py-5"><ErrorBoundary resetKey={pathname}><Suspense fallback={<SkeletonRows rows={8} />}><Page pathname={pathname} /></Suspense></ErrorBoundary></div>
        </main>
      </div>
    </div>
  )
}
