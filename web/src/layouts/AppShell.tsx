import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Bell, ChevronDown, GitCompareArrows, ChevronRight, LogOut, PanelLeftClose, PanelLeftOpen, Search } from 'lucide-react'
import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router'
import { get } from '../lib/api'
import { signOut } from '../lib/auth'
import { allItems, nav } from '../lib/nav'
import { useCan, useMe } from '../lib/session'
import type { Summary } from '../lib/types'

function LogoMark() {
  return (
    <svg width="26" height="26" viewBox="0 0 32 32" aria-hidden className="shrink-0">
      <rect width="32" height="32" rx="7" fill="#17305f" />
      <rect x="7" y="7" width="8" height="8" rx="1.5" fill="#6c9bf2" />
      <rect x="17" y="17" width="8" height="8" rx="1.5" fill="#6c9bf2" />
      <rect x="17" y="7" width="8" height="8" rx="1.5" fill="#fff" opacity=".55" />
      <rect x="7" y="17" width="8" height="8" rx="1.5" fill="#fff" opacity=".55" />
    </svg>
  )
}

function Sidebar({ collapsed, onToggle }: { collapsed: boolean; onToggle: () => void }) {
  return (
    <aside className={`flex shrink-0 flex-col border-r border-side-border bg-side transition-[width] duration-150 ${collapsed ? 'w-[60px]' : 'w-[248px]'}`}>
      <div className={`flex h-[60px] shrink-0 items-center border-b border-side-border ${collapsed ? 'justify-center' : 'justify-between pr-2 pl-4'}`}>
        <Link to="/" aria-label="DF-IPAM home" className="flex min-w-0 items-center gap-2.5">
          <LogoMark />
          {!collapsed && (
            <span className="min-w-0 leading-[1.15]">
              <span className="block text-[15px] font-semibold tracking-tight text-side-ink">DF-IPAM</span>
              <span className="block truncate text-[11px] text-side-ink-3">Dialog Finance</span>
            </span>
          )}
        </Link>
        {!collapsed && (
          <button type="button" onClick={onToggle} aria-label="Collapse sidebar" className="grid size-7 cursor-pointer place-items-center rounded text-side-ink-3 hover:bg-side-hover hover:text-side-ink">
            <PanelLeftClose size={16} />
          </button>
        )}
      </div>

      <nav aria-label="Main" className="flex-1 overflow-y-auto py-2">
        {collapsed && (
          <button type="button" onClick={onToggle} aria-label="Expand sidebar" className="mx-auto mb-1 grid size-8 cursor-pointer place-items-center rounded text-side-ink-3 hover:bg-side-hover hover:text-side-ink">
            <PanelLeftOpen size={16} />
          </button>
        )}
        {nav.map(({ group, items }) => (
          <div key={group} className="mb-2.5">
            {collapsed ? <div className="mx-4 my-2 h-px bg-side-border" />
              : <div className="px-4 pt-1.5 pb-1 text-[10.5px] font-semibold tracking-[.08em] text-side-ink-3 uppercase">{group}</div>}
            {items.map(({ to, label, icon: Icon, phase }) => (
              <NavLink
                key={to}
                to={to}
                end={to === '/'}
                title={collapsed ? label : undefined}
                aria-label={label}
                className={({ isActive }) =>
                  `relative flex h-[30px] items-center gap-2.5 text-[13px] transition-colors ${collapsed ? 'justify-center' : 'pr-3 pl-4'} ${
                    isActive
                      ? 'bg-side-active font-medium text-side-active-ink before:absolute before:inset-y-0 before:left-0 before:w-[3px] before:bg-accent'
                      : `hover:bg-side-hover hover:text-side-ink ${phase ? 'text-side-ink-3' : 'text-side-ink-2'}`
                  }`
                }
              >
                <Icon size={15} strokeWidth={1.75} className="shrink-0" aria-hidden />
                {!collapsed && <span className="min-w-0 flex-1 truncate">{label}</span>}
                {!collapsed && phase && <span className="text-[10px] text-side-ink-3">Planned</span>}
              </NavLink>
            ))}
          </div>
        ))}
      </nav>

    </aside>
  )
}


function GlobalSearch() {
  const navigate = useNavigate()
  const input = useRef<HTMLInputElement>(null)
  const [value, setValue] = useState('')

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        input.current?.focus()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const q = value.trim()
    if (q) navigate(`/search?q=${encodeURIComponent(q)}`)
  }

  return (
    <form role="search" onSubmit={submit} className="relative w-full max-w-md">
      <Search size={14} className="absolute top-1/2 left-3 -translate-y-1/2 text-ink-3" aria-hidden />
      <input
        ref={input}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        aria-label="Search the inventory"
        placeholder="Search IPs, subnets, hostnames, MACs, devices"
        className="h-[34px] w-full rounded-[5px] border border-transparent bg-bg pr-16 pl-8.5 text-[13px] text-ink placeholder:text-ink-3 focus:border-accent focus:bg-surface focus:outline-none"
      />
      <kbd className="absolute top-1/2 right-2 -translate-y-1/2 rounded-[3px] border border-line-strong bg-surface px-1.5 font-sans text-[10.5px] text-ink-3 max-lg:hidden">Ctrl K</kbd>
    </form>
  )
}

function FindingsLink() {
  const canView = useCan('inventory:view')
  const { data } = useQuery({ queryKey: ['summary'], enabled: canView, queryFn: () => get<Summary>('/dashboard/summary') })
  const open = data?.openFindings ?? 0
  if (!canView) return null
  return (
    <Link to="/reconciliation" aria-label={`${open} open findings`} title="Open findings"
      className="relative grid size-8 place-items-center rounded-[5px] text-ink-2 hover:bg-bg hover:text-ink">
      <GitCompareArrows size={16} strokeWidth={1.75} />
      {open > 0 && <span className="absolute -top-0.5 -right-0.5 min-w-4 rounded-full bg-warn px-1 text-center text-[10px] leading-4 font-semibold text-white tabular-nums">{open > 99 ? '99+' : open}</span>}
    </Link>
  )
}

/** Unread alerts for the signed-in user, refreshed every minute. */
function AlertsBell() {
  const { data } = useQuery({ queryKey: ['notifications', 'unread'], refetchInterval: 60_000, queryFn: () => get<{ unread: number }>('/notifications/unread-count') })
  const unread = data?.unread ?? 0
  return (
    <Link to="/alerts" aria-label={`${unread} unread alerts`} title="Alerts"
      className="relative grid size-8 place-items-center rounded-[5px] text-ink-2 hover:bg-bg hover:text-ink">
      <Bell size={16} strokeWidth={1.75} />
      {unread > 0 && <span className="absolute -top-0.5 -right-0.5 min-w-4 rounded-full bg-danger px-1 text-center text-[10px] leading-4 font-semibold text-white tabular-nums">{unread > 99 ? '99+' : unread}</span>}
    </Link>
  )
}

function UserMenu() {
  const me = useMe()
  const queryClient = useQueryClient()
  const role = me.roles[0] ?? 'No role assigned'
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger aria-label="Account menu"
        className="flex cursor-pointer items-center gap-2 rounded-[5px] py-1 pr-1.5 pl-1 text-left hover:bg-bg data-[state=open]:bg-bg">
        <span aria-hidden className="grid size-[30px] shrink-0 place-items-center rounded-[5px] bg-accent text-[12.5px] font-semibold text-white">{me.email.charAt(0).toUpperCase()}</span>
        <span className="min-w-0 leading-tight max-md:hidden">
          <span className="block max-w-44 truncate text-[12.5px] font-medium text-ink">{me.email}</span>
          <span className="block max-w-44 truncate text-[11px] text-ink-3">{role}</span>
        </span>
        <ChevronDown size={14} className="shrink-0 text-ink-3" aria-hidden />
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="end" sideOffset={6}
          className="z-30 w-64 rounded-md border border-line-strong/70 bg-surface p-1 shadow-[0_12px_32px_-12px_rgba(11,18,32,.3)]">
          <div className="border-b border-line px-2.5 pt-1.5 pb-2 leading-tight">
            <div className="truncate text-[13px] font-medium text-ink">{me.email}</div>
            <div className="mt-0.5 truncate text-[12px] text-ink-3">{me.roles.join(', ') || role}</div>
          </div>
          <DropdownMenu.Item onSelect={async () => { await signOut(); queryClient.clear() }}
            className="mt-1 flex h-8 cursor-pointer items-center gap-2 rounded-[4px] px-2.5 text-[13px] text-ink-2 outline-none data-[highlighted]:bg-bg data-[highlighted]:text-ink">
            <LogOut size={14} aria-hidden /> Sign out
          </DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

export function AppShell() {
  const { pathname } = useLocation()
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem('dfipam.sidebar') === 'collapsed' || window.innerWidth < 1024)
  const toggle = () => setCollapsed((c) => {
    localStorage.setItem('dfipam.sidebar', c ? 'open' : 'collapsed')
    return !c
  })
  const current = allItems.find((item) => (item.to === '/' ? pathname === '/' : pathname.startsWith(item.to)))
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
          <div className="mx-auto max-w-[1440px] px-6 py-5"><Outlet /></div>
        </main>
      </div>
    </div>
  )
}
