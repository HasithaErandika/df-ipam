import { useQuery } from '@tanstack/react-query'
import { Check, ChevronDown, Loader2, X } from 'lucide-react'
import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { listAll } from '../lib/api'

export interface Option { value: string; label: string; detail?: string }

/** Long lists stay quick: the rest are one search away. */
const shown = 100

interface Props {
  id?: string
  value: string
  onChange: (value: string) => void
  options: Option[]
  /** Offers a choice that clears the value, such as "None" or "All". */
  emptyLabel?: string
  placeholder?: string
  loading?: boolean
  fetchError?: unknown
  onRetry?: () => void
  disabled?: boolean
  className?: string
  'aria-label'?: string
  'aria-invalid'?: boolean
  'aria-describedby'?: string
}

const control = 'h-8 w-full rounded-[5px] border border-line-strong bg-surface-2 pr-14 pl-2.5 text-[13px] text-ink placeholder:text-ink-3 focus:border-accent focus:bg-surface focus:outline-none disabled:opacity-55 aria-invalid:border-danger'

/**
 * A select you can type into. In a dialog the list opens in the page flow, because
 * the dialog's scrolling body would otherwise clip it.
 */
export function SearchSelect({ id, value, onChange, options, emptyLabel, placeholder, loading, fetchError, onRetry, disabled, className, ...aria }: Props) {
  const box = useRef<HTMLDivElement>(null)
  const input = useRef<HTMLInputElement>(null)
  const list = useRef<HTMLUListElement>(null)
  const listId = useId()
  const [open, setOpen] = useState(false)
  const [text, setText] = useState('')
  const [active, setActive] = useState(0)
  const [inline, setInline] = useState(false)
  const selected = options.find((o) => o.value === value)

  const matches = useMemo(() => {
    const words = text.trim().toLowerCase().split(/\s+/).filter(Boolean)
    const found = words.length === 0 ? options : options.filter((o) => {
      const hay = `${o.label} ${o.detail ?? ''}`.toLowerCase()
      return words.every((w) => hay.includes(w))
    })
    return found
  }, [options, text])
  const items: Option[] = [...(emptyLabel && !text.trim() ? [{ value: '', label: emptyLabel }] : []), ...matches.slice(0, shown)]

  useEffect(() => {
    if (!open) return
    const close = (e: PointerEvent) => { if (!box.current?.contains(e.target as Node)) setOpen(false) }
    document.addEventListener('pointerdown', close)
    return () => document.removeEventListener('pointerdown', close)
  }, [open])
  useEffect(() => { list.current?.querySelector('[data-active="true"]')?.scrollIntoView({ block: 'nearest' }) }, [active, open])

  const show = () => {
    if (disabled) return
    setInline(Boolean(box.current?.closest('[role="dialog"]')))
    setText('')
    setActive(Math.max(0, items.findIndex((o) => o.value === value)))
    setOpen(true)
  }
  const pick = (o: Option) => { onChange(o.value); setOpen(false); setText('') }
  const onKey = (e: KeyboardEvent) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault()
      if (!open) return show()
      setActive((i) => Math.min(Math.max(i + (e.key === 'ArrowDown' ? 1 : -1), 0), Math.max(items.length - 1, 0)))
    } else if (e.key === 'Enter' && open) {
      e.preventDefault() // choosing an option mustn't submit the form
      if (items[active]) pick(items[active])
    } else if (e.key === 'Escape' && open) {
      e.preventDefault()
      e.stopPropagation() // closes this list, not the dialog around it
      setOpen(false)
    } else if (e.key === 'Tab') {
      setOpen(false)
    }
  }
  const hidden = matches.length - Math.min(matches.length, shown)
  const failed = Boolean(fetchError)
  return (
    <div ref={box} className={`relative ${className ?? ''}`}>
      <input ref={input} id={id} role="combobox" aria-expanded={open} aria-controls={listId} aria-autocomplete="list" autoComplete="off" spellCheck={false}
        aria-activedescendant={open && items[active] ? `${listId}-${active}` : undefined} {...aria} disabled={disabled} className={control}
        placeholder={loading ? 'Loading…' : failed ? 'Could not load options' : (placeholder ?? emptyLabel ?? 'Select')}
        value={open ? text : (selected?.label ?? '')}
        onFocus={show} onClick={() => { if (!open) show() }} onKeyDown={onKey}
        onChange={(e) => { setText(e.target.value); setActive(0); if (!open) setOpen(true) }} />
      <span className="absolute inset-y-0 right-1.5 flex items-center gap-0.5 text-ink-3">
        {loading && <Loader2 size={13} className="animate-spin" aria-hidden />}
        {value && emptyLabel !== undefined && !disabled && (
          <button type="button" aria-label="Clear" className="cursor-pointer rounded p-0.5 hover:text-danger" onClick={() => { onChange(''); input.current?.focus() }}><X size={13} /></button>
        )}
        <ChevronDown size={14} aria-hidden />
      </span>
      {open && (
        <div className={`${inline ? 'mt-1' : 'absolute z-30 mt-1 w-full min-w-56 shadow-[0_8px_24px_-8px_rgba(11,18,32,.25)]'} overflow-hidden rounded-[5px] border border-line-strong bg-surface`}>
          <ul ref={list} id={listId} role="listbox" className="max-h-52 overflow-y-auto py-1">
            {failed && (
              <li className="flex items-center justify-between gap-2 px-2.5 py-1.5 text-[13px] text-danger">
                Couldn't load options.
                {onRetry && <button type="button" className="cursor-pointer text-accent hover:underline" onClick={onRetry}>Try again</button>}
              </li>
            )}
            {!failed && loading && <li className="px-2.5 py-1.5 text-[13px] text-ink-3">Loading…</li>}
            {!failed && !loading && items.length === 0 && <li className="px-2.5 py-1.5 text-[13px] text-ink-3">{text.trim() ? 'No matches' : 'Nothing to choose yet'}</li>}
            {items.map((o, i) => (
              <li key={o.value || 'empty'} id={`${listId}-${i}`} role="option" aria-selected={o.value === value} data-active={i === active}
                onPointerDown={(e) => e.preventDefault()} onClick={() => pick(o)} onPointerEnter={() => setActive(i)}
                className={`flex cursor-pointer items-center justify-between gap-2 px-2.5 py-1 text-[13px] ${i === active ? 'bg-accent-soft text-accent' : 'text-ink'}`}>
                <span className="min-w-0 truncate">{o.label}{o.detail && <span className="ml-2 text-xs text-ink-3">{o.detail}</span>}</span>
                {o.value === value && <Check size={13} className="shrink-0" aria-hidden />}
              </li>
            ))}
            {hidden > 0 && <li className="px-2.5 py-1 text-xs text-ink-3">{hidden} more. Keep typing to narrow the list.</li>}
          </ul>
        </div>
      )}
    </div>
  )
}

interface RefProps<T> extends Omit<Props, 'options' | 'loading' | 'fetchError' | 'onRetry'> {
  /** The list endpoint. Every page is loaded, so no option is silently missing. */
  path: string
  toOption: (row: T) => Option
  filter?: (row: T) => boolean
}

/** A SearchSelect that fetches its own options. */
export function RefSelect<T>({ path, toOption, filter, ...rest }: RefProps<T>) {
  const q = useQuery({ queryKey: ['options', path], queryFn: () => listAll<T>(path) })
  const rows = filter ? q.data?.filter(filter) : q.data
  return <SearchSelect {...rest} options={rows?.map(toOption) ?? []} loading={q.isPending} fetchError={q.error} onRetry={() => q.refetch()} />
}

interface DomainRow { id: string; organisationName: string; name: string }
const domainOption = (d: DomainRow): Option => ({ value: d.id, label: `${d.organisationName} / ${d.name}` })

type Picker = Omit<RefProps<never>, 'path' | 'toOption' | 'filter'>

export const RoutingDomainSelect = (p: Picker) => <RefSelect<DomainRow> {...p} path="/routing-domains" toOption={domainOption} />

export const PersonSelect = (p: Picker) => (
  <RefSelect<{ id: string; email: string; displayName: string }> {...p} path="/people" toOption={(u) => ({ value: u.id, label: u.email, detail: u.displayName })} />
)

/** Time zones the browser knows, which are the same IANA names the API accepts. */
export function timezones(): Option[] {
  return Intl.supportedValuesOf('timeZone').map((z) => ({ value: z, label: z }))
}
