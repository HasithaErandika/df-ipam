import { useQuery } from '@tanstack/react-query'
import { Search } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { list, queryString } from '../lib/api'
import { requestStatuses, requestTone } from '../lib/format'
import type { Subnet } from '../lib/types'
import { Badge, Input, Mono } from './ui'

export function useDebounced<T>(value: T, ms = 300): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return debounced
}

export function StatusBadge({ status }: { status: string }) {
  return <Badge tone={requestTone(status)}>{requestStatuses[status] ?? status}</Badge>
}

export function Detail({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex justify-between gap-4 border-b border-line py-1.5 text-[13px] last:border-0">
      <dt className="shrink-0 text-ink-2">{label}</dt>
      <dd className="min-w-0 text-right break-words text-ink">{children}</dd>
    </div>
  )
}

export interface TabDef { key: string; label: string; count?: number }

export function Tabs({ tabs, active, onChange }: { tabs: TabDef[]; active: string; onChange: (key: string) => void }) {
  return (
    <div role="tablist" className="mb-3 flex flex-wrap gap-1 border-b border-line">
      {tabs.map((t) => (
        <button key={t.key} type="button" role="tab" aria-selected={active === t.key} onClick={() => onChange(t.key)}
          className={`-mb-px flex h-9 cursor-pointer items-center gap-1.5 border-b-2 px-3 text-[13px] font-medium ${active === t.key ? 'border-accent text-accent' : 'border-transparent text-ink-2 hover:text-ink'}`}>
          {t.label}
          {t.count !== undefined && t.count > 0 && <span className="rounded-full bg-accent px-1.5 text-[10.5px] leading-4 font-semibold text-white tabular-nums">{t.count}</span>}
        </button>
      ))}
    </div>
  )
}

/**
 * Picks a subnet by searching name or CIDR. Inventories hold thousands of
 * subnets, so it searches on the server instead of loading them all.
 */
export function SubnetPicker({ id, value, onChange, invalid }:
  { id: string; value: Subnet | null; onChange: (s: Subnet | null) => void; invalid?: boolean }) {
  const [text, setText] = useState('')
  const q = useDebounced(text.trim())
  const results = useQuery({
    queryKey: ['subnet-picker', q],
    enabled: !value,
    queryFn: () => list<Subnet>(`/subnets${queryString({ q, pageSize: 12, sort: 'cidr' })}`),
  })
  if (value) {
    return (
      <div className="flex h-8 items-center justify-between gap-2 rounded-[5px] border border-line-strong bg-surface-2 px-2.5 text-[13px]">
        <span className="min-w-0 truncate"><Mono>{value.calculated.cidr}</Mono> <span className="text-ink-2">{value.name} · {value.routingDomainName}</span></span>
        <button type="button" className="cursor-pointer text-[12.5px] text-accent hover:underline" onClick={() => onChange(null)}>Change</button>
      </div>
    )
  }
  return (
    <div>
      <div className="relative">
        <Search size={14} className="absolute top-1/2 left-2.5 -translate-y-1/2 text-ink-3" aria-hidden />
        <Input id={id} aria-invalid={invalid} className="pl-8" placeholder="Search by name or CIDR" value={text} onChange={(e) => setText(e.target.value)} />
      </div>
      <ul className="mt-1 max-h-48 overflow-y-auto rounded-[5px] border border-line" aria-label="Matching subnets">
        {results.isPending && <li className="px-2.5 py-1.5 text-[13px] text-ink-3">Loading…</li>}
        {results.error && <li className="px-2.5 py-1.5 text-[13px] text-danger">Subnets couldn't be loaded.</li>}
        {results.data?.data.length === 0 && <li className="px-2.5 py-1.5 text-[13px] text-ink-3">No active subnets match.</li>}
        {results.data?.data.map((s) => (
          <li key={s.id}>
            <button type="button" onClick={() => onChange(s)}
              className="flex w-full cursor-pointer items-center justify-between gap-3 px-2.5 py-1.5 text-left text-[13px] hover:bg-surface-2">
              <span className="min-w-0 truncate"><Mono>{s.calculated.cidr}</Mono> <span className="text-ink-2">{s.name}</span></span>
              <span className="shrink-0 text-[12px] text-ink-3">{s.routingDomainName} · {s.utilisationPct}% used</span>
            </button>
          </li>
        ))}
      </ul>
    </div>
  )
}
