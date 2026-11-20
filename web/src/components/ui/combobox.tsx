import { ChevronDown, X } from 'lucide-react'
import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { cx } from './cx'

export interface ComboOption { value: string; label: string; detail?: string }

const shown = 100

/**
 * A dropdown that can be searched by typing, for lists too long to scroll
 * (people, subnets, devices). Arrow keys move, Enter picks, Escape closes.
 */
export function SearchSelect({ id, value, onChange, options, placeholder = 'Search…', emptyLabel = 'None', allowEmpty = true, invalid }: {
  id: string; value: string; onChange: (value: string) => void; options: ComboOption[]
  placeholder?: string; emptyLabel?: string; allowEmpty?: boolean; invalid?: boolean
}) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const box = useRef<HTMLDivElement>(null)
  const listId = useId()
  const selected = options.find((o) => o.value === value)
  const matches = useMemo(() => {
    const q = query.trim().toLowerCase()
    const hits = q ? options.filter((o) => o.label.toLowerCase().includes(q) || o.detail?.toLowerCase().includes(q)) : options
    return hits.slice(0, shown)
  }, [options, query])

  useEffect(() => {
    if (!open) return
    const away = (e: MouseEvent) => { if (!box.current?.contains(e.target as Node)) setOpen(false) }
    document.addEventListener('mousedown', away)
    return () => document.removeEventListener('mousedown', away)
  }, [open])

  const pick = (v: string) => { onChange(v); setOpen(false); setQuery('') }
  const key = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setOpen(true); setActive((a) => Math.min(a + 1, matches.length - 1)) }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setActive((a) => Math.max(a - 1, 0)) }
    else if (e.key === 'Enter' && open && matches[active]) { e.preventDefault(); pick(matches[active].value) }
    else if (e.key === 'Escape') setOpen(false)
  }

  return (
    <div ref={box} className="relative">
      <div className={cx('flex h-8 items-center rounded-[5px] border bg-surface-2 pr-1 focus-within:border-accent focus-within:bg-surface', invalid ? 'border-danger' : 'border-line-strong')}>
        <input id={id} role="combobox" aria-expanded={open} aria-controls={listId} aria-autocomplete="list" autoComplete="off"
          className="h-full min-w-0 flex-1 bg-transparent px-2.5 text-[13px] text-ink outline-none placeholder:text-ink-3"
          placeholder={selected ? selected.label : placeholder} value={open ? query : selected?.label ?? ''}
          onFocus={() => { setOpen(true); setActive(0) }} onChange={(e) => { setQuery(e.target.value); setOpen(true); setActive(0) }} onKeyDown={key} />
        {allowEmpty && value && <button type="button" aria-label="Clear" className="cursor-pointer p-1 text-ink-3 hover:text-ink" onClick={() => pick('')}><X size={14} /></button>}
        <ChevronDown size={15} aria-hidden className="text-ink-3" />
      </div>
      {open && (
        <ul id={listId} role="listbox" className="absolute z-30 mt-1 max-h-64 w-full overflow-y-auto rounded-md border border-line-strong/70 bg-surface p-1 shadow-[0_12px_32px_-12px_rgba(11,18,32,.3)]">
          {allowEmpty && !query && <li role="option" aria-selected={!value} className="cursor-pointer rounded-[4px] px-2.5 py-1.5 text-[13px] text-ink-3 hover:bg-bg" onMouseDown={(e) => { e.preventDefault(); pick('') }}>{emptyLabel}</li>}
          {matches.length === 0 && <li className="px-2.5 py-1.5 text-[13px] text-ink-3">Nothing matches.</li>}
          {matches.map((o, i) => (
            <li key={o.value} role="option" aria-selected={o.value === value}
              className={cx('cursor-pointer rounded-[4px] px-2.5 py-1.5 text-[13px]', i === active ? 'bg-accent-soft text-ink' : 'text-ink-2 hover:bg-bg', o.value === value && 'font-medium')}
              onMouseEnter={() => setActive(i)} onMouseDown={(e) => { e.preventDefault(); pick(o.value) }}>
              <span className="block truncate">{o.label}</span>
              {o.detail && <span className="block truncate text-[11.5px] text-ink-3">{o.detail}</span>}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

/** Several picks from a searchable list, shown as removable chips. */
export function MultiSearchSelect({ id, values, onChange, options, placeholder }: {
  id: string; values: string[]; onChange: (values: string[]) => void; options: ComboOption[]; placeholder?: string
}) {
  const left = options.filter((o) => !values.includes(o.value))
  return (
    <div className="space-y-1.5">
      {values.length > 0 && (
        <ul className="flex flex-wrap gap-1.5">
          {values.map((v) => {
            const o = options.find((x) => x.value === v)
            return (
              <li key={v} className="inline-flex items-center gap-1 rounded-[4px] border border-line bg-surface-2 py-0.5 pr-1 pl-2 text-[12.5px] text-ink">
                {o?.label ?? v}{o?.detail && <span className="text-ink-3"> {o.detail}</span>}
                <button type="button" aria-label={`Remove ${o?.label ?? v}`} className="cursor-pointer p-0.5 text-ink-3 hover:text-danger" onClick={() => onChange(values.filter((x) => x !== v))}><X size={12} /></button>
              </li>
            )
          })}
        </ul>
      )}
      <SearchSelect id={id} value="" options={left} placeholder={placeholder} allowEmpty={false} onChange={(v) => v && onChange([...values, v])} />
    </div>
  )
}
