import { Search, X } from 'lucide-react'
import { type ReactNode, useEffect, useState } from 'react'
import { useDebounced } from '../../lib/listing'
import { cx } from './cx'

/** The row of filters above a table. */
export function FilterBar({ children, active, onClear, end }: { children: ReactNode; active?: number; onClear?: () => void; end?: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center gap-2 border-b border-line px-3 py-2.5">
      {children}
      {active ? (
        <button type="button" onClick={onClear} className="inline-flex h-8 cursor-pointer items-center gap-1 rounded-[5px] px-2 text-[12.5px] font-medium text-ink-2 hover:bg-surface-2 hover:text-ink">
          <X size={13} aria-hidden /> Clear {active > 1 ? `${active} filters` : 'filter'}
        </button>
      ) : null}
      {end && <div className="ml-auto flex items-center gap-2">{end}</div>}
    </div>
  )
}

/**
 * Search that waits for typing to pause before searching, so a word sends one
 * request rather than one per key.
 */
export function SearchInput({ value, onChange, placeholder, className }: { value: string; onChange: (v: string) => void; placeholder: string; className?: string }) {
  const [draft, setDraft] = useState(value)
  const settled = useDebounced(draft, 350)
  useEffect(() => { setDraft(value) }, [value])
  useEffect(() => {
    if (settled !== value) onChange(settled.trim().slice(0, 100))
    // onChange changes every render; only the settled text should trigger this.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [settled])
  return (
    <label className={cx('relative flex h-8 w-64 items-center max-sm:w-full', className)}>
      <span className="sr-only">{placeholder}</span>
      <Search size={14} className="pointer-events-none absolute left-2.5 text-ink-3" aria-hidden />
      <input type="search" value={draft} onChange={(e) => setDraft(e.target.value)} placeholder={placeholder} maxLength={100}
        className="h-8 w-full rounded-[5px] border border-line-strong bg-surface px-2 pl-8 text-[13px] text-ink placeholder:text-ink-3 focus:border-accent focus:outline-none" />
    </label>
  )
}

/** A compact select for a filter. An empty value means "any". */
export function FilterSelect({ label, value, onChange, options, anyLabel }: {
  label: string; value: string; onChange: (v: string) => void; options: { value: string; text: string }[]; anyLabel?: string
}) {
  return (
    <label className="flex h-8 items-center">
      <span className="sr-only">{label}</span>
      <select value={value} onChange={(e) => onChange(e.target.value)} aria-label={label}
        className={cx('h-8 cursor-pointer rounded-[5px] border px-2 pr-7 text-[13px] focus:border-accent focus:outline-none',
          value ? 'border-accent/40 bg-accent-soft text-accent' : 'border-line-strong bg-surface text-ink-2')}>
        <option value="">{anyLabel ?? `Any ${label.toLowerCase()}`}</option>
        {options.map((o) => <option key={o.value} value={o.value}>{o.text}</option>)}
      </select>
    </label>
  )
}

/** Mutually exclusive choices shown as a segmented control, such as a time window. */
export function Segmented({ label, value, onChange, options }: { label: string; value: string; onChange: (v: string) => void; options: { value: string; text: string }[] }) {
  return (
    <div role="radiogroup" aria-label={label} className="inline-flex h-8 items-center rounded-[5px] border border-line-strong bg-surface-2 p-0.5">
      {options.map((o) => (
        <button key={o.value} type="button" role="radio" aria-checked={value === o.value} onClick={() => onChange(o.value)}
          className={cx('h-[26px] cursor-pointer rounded-[4px] px-2.5 text-[12.5px] font-medium',
            value === o.value ? 'bg-surface text-ink shadow-pill' : 'text-ink-3 hover:text-ink')}>
          {o.text}
        </button>
      ))}
    </div>
  )
}
