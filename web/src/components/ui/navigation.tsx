import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { MoreHorizontal } from 'lucide-react'
import { type ReactNode } from 'react'
import { NavLink, useLocation } from 'react-router'
import { tabPath } from '../../lib/tabs'
import { cx } from './cx'

export interface TabItem { key: string; label: string; count?: number }

/** Tabs that are paths, so each can be linked to. The query string is kept for filters that still apply. */
export function PathTabs({ base, tabs, keepQuery = false }: { base: string; tabs: TabItem[]; keepQuery?: boolean }) {
  const { search } = useLocation()
  return (
    <nav aria-label="Sections" className="mb-4 flex gap-1 overflow-x-auto border-b border-line">
      {tabs.map((t) => (
        <NavLink key={t.key} to={tabPath(base, t.key) + (keepQuery ? search : '')} end
          className={({ isActive }) => cx('-mb-px inline-flex h-9 shrink-0 items-center gap-1.5 border-b-2 px-3 text-[13px] font-medium whitespace-nowrap',
            isActive ? 'border-accent text-accent' : 'border-transparent text-ink-2 hover:text-ink')}>
          {t.label}
          {t.count ? <span className="rounded-full bg-surface-2 px-1.5 text-[11px] text-ink-2 tabular-nums">{t.count}</span> : null}
        </NavLink>
      ))}
    </nav>
  )
}

export interface MenuAction { label: string; onSelect: () => void; danger?: boolean; icon?: ReactNode; hidden?: boolean }

/** The "…" menu at the end of a row, for actions that would crowd it. */
export function RowMenu({ actions, label = 'Actions' }: { actions: MenuAction[]; label?: string }) {
  const shown = actions.filter((a) => !a.hidden)
  if (shown.length === 0) return null
  return (
    <DropdownMenu.Root modal={false}>
      <DropdownMenu.Trigger aria-label={label} className="grid size-7 cursor-pointer place-items-center rounded-[4px] text-ink-3 hover:bg-surface-2 hover:text-ink data-[state=open]:bg-surface-2 data-[state=open]:text-ink">
        <MoreHorizontal size={16} aria-hidden />
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="end" sideOffset={4} className="z-30 min-w-44 rounded-md border border-line-strong/70 bg-surface p-1 shadow-[0_12px_32px_-12px_rgba(11,18,32,.3)]">
          {shown.map((a) => (
            <DropdownMenu.Item key={a.label} onSelect={a.onSelect}
              className={cx('flex h-8 cursor-pointer items-center gap-2 rounded-[4px] px-2.5 text-[13px] outline-none data-[highlighted]:bg-bg',
                a.danger ? 'text-danger' : 'text-ink-2 data-[highlighted]:text-ink')}>
              {a.icon}{a.label}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}
