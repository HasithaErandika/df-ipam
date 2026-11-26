import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { Check, Columns3 } from 'lucide-react'
import { useState } from 'react'
import { Button } from '../../components/ui'

const item = 'flex h-8 cursor-pointer items-center gap-2 rounded-[4px] px-2.5 text-[13px] text-ink-2 outline-none data-[highlighted]:bg-bg data-[highlighted]:text-ink'

/** Which optional columns a list shows, remembered in this browser only. */
export function useColumnChoice(storageKey: string, initial: string[]) {
  const [shown, setShown] = useState<string[]>(() => {
    try {
      const saved = JSON.parse(localStorage.getItem(storageKey) ?? 'null') as unknown
      return Array.isArray(saved) ? saved.filter((x): x is string => typeof x === 'string') : initial
    } catch {
      return initial
    }
  })
  const toggle = (key: string) => setShown((prev) => {
    const next = prev.includes(key) ? prev.filter((k) => k !== key) : [...prev, key]
    try { localStorage.setItem(storageKey, JSON.stringify(next)) } catch { /* not remembered */ }
    return next
  })
  return { shown, toggle }
}

/** A menu of optional columns to show or hide. */
export function ColumnChooser({ columns, shown, onToggle }: { columns: { key: string; label: string }[]; shown: string[]; onToggle: (key: string) => void }) {
  return (
    <DropdownMenu.Root modal={false}>
      <DropdownMenu.Trigger asChild><Button><Columns3 size={14} aria-hidden />Columns</Button></DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="end" sideOffset={4} className="z-30 min-w-48 rounded-md border border-line-strong/70 bg-surface p-1 shadow-menu">
          {columns.map((c) => (
            <DropdownMenu.CheckboxItem key={c.key} className={item} checked={shown.includes(c.key)} onCheckedChange={() => onToggle(c.key)} onSelect={(e) => e.preventDefault()}>
              <span className="grid w-4 place-items-center">{shown.includes(c.key) && <Check size={14} aria-hidden />}</span>{c.label}
            </DropdownMenu.CheckboxItem>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}
