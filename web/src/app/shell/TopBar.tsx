import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Bell, GitCompareArrows, IdCard, LogOut, Search, UserRound } from 'lucide-react'
import { type FormEvent, useEffect, useRef, useState } from 'react'
import { Link, useNavigate } from 'react-router'
import { get } from '../../lib/api'
import { signOut } from '../../lib/auth'
import { useCan } from '../../lib/session'
import type { Summary } from '../../lib/types'
import { ProfileModal } from './ProfileModal'

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

const menuItem = 'flex h-8 cursor-pointer items-center gap-2 rounded-[4px] px-2.5 text-[13px] text-ink-2 outline-none data-[highlighted]:bg-bg data-[highlighted]:text-ink'

export function UserMenu() {
  const queryClient = useQueryClient()
  const [details, setDetails] = useState(false)
  return (
    <>
      {/* Not modal: a modal menu would fight the details dialog for focus as it closes. */}
      <DropdownMenu.Root modal={false}>
        <DropdownMenu.Trigger aria-label="Account menu" title="Account"
          className="grid size-8 cursor-pointer place-items-center rounded-full bg-accent-soft text-accent hover:bg-accent hover:text-white data-[state=open]:bg-accent data-[state=open]:text-white">
          <UserRound size={17} strokeWidth={1.75} aria-hidden />
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content align="end" sideOffset={6}
            className="z-30 w-44 rounded-md border border-line-strong/70 bg-surface p-1 shadow-[0_12px_32px_-12px_rgba(11,18,32,.3)]">
            <DropdownMenu.Item onSelect={() => setDetails(true)} className={menuItem}>
              <IdCard size={14} aria-hidden /> Details
            </DropdownMenu.Item>
            <DropdownMenu.Item onSelect={async () => { await signOut(); queryClient.clear() }} className={menuItem}>
              <LogOut size={14} aria-hidden /> Sign out
            </DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
      <ProfileModal open={details} onClose={() => setDetails(false)} />
    </>
  )
}
