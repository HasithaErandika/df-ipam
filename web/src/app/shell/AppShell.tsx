import { ChevronRight } from 'lucide-react'
import { Suspense, useEffect, useState } from 'react'
import { Navigate, Outlet, useLocation } from 'react-router'
import { legacyTabUrl } from '../../lib/tabs'
import { Sidebar } from './Sidebar'
import { AlertsBell, FindingsLink, GlobalSearch, UserMenu } from './TopBar'
import { ErrorBoundary, SkeletonRows } from '../../components/ui'
import { allItems } from '../nav'

export function AppShell() {
  const { pathname, search } = useLocation()
  const legacyTab = legacyTabUrl(pathname, search)
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
        <header className="grid h-[60px] shrink-0 grid-cols-[1fr_minmax(0,28rem)_1fr] items-center gap-4 border-b border-line bg-surface px-6">
          <nav aria-label="Breadcrumb" className="flex min-w-0 items-center gap-1 text-[12.5px] text-ink-3">
            <span className="truncate max-md:hidden">{current?.group}</span>
            <ChevronRight size={13} className="shrink-0 max-md:hidden" aria-hidden />
            <span aria-current="page" className="truncate font-medium text-ink">{current?.label}</span>
          </nav>
          <GlobalSearch />
          <div className="flex items-center justify-end gap-2">
            <FindingsLink />
            <AlertsBell />
            <UserMenu />
          </div>
        </header>
        <main className="min-h-0 flex-1 overflow-y-auto">
          <div className="mx-auto max-w-[1440px] px-6 py-5"><ErrorBoundary resetKey={pathname}><Suspense fallback={<SkeletonRows rows={8} />}>{legacyTab ? <Navigate to={legacyTab} replace /> : <Outlet />}</Suspense></ErrorBoundary></div>
        </main>
      </div>
    </div>
  )
}
