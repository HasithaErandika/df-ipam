import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Bell, ChevronDown, GitCompareArrows, LogOut, Search } from 'lucide-react'
import { type FormEvent, useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router'
import { get } from '../../lib/api'
import { signOut } from '../../lib/auth'
import { useCan, useMe } from '../../lib/session'
import type { Summary } from '../../lib/types'

export function GlobalSearch() {
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

export function FindingsLink() {
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
export function AlertsBell() {
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

export function UserMenu() {
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
