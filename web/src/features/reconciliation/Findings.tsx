import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { FindingModal } from './FindingModal'
import { RunPanel } from './RunPanel'
import { Badge, Button, Card, EmptyState, ErrorState, Input, Mono, Pagination, Select, SkeletonRows, Td, Th } from '../../components/ui'
import { useDebounced } from '../../components/workflow'
import { get, list, queryString } from '../../lib/api'
import { classifications, formatAgo, formatDateTime } from '../../lib/format'
import { useClampPage, usePaging } from '../../lib/paging'
import type { Finding, FindingCount } from '../../lib/types'

const statuses: Record<string, string> = {
  active: 'Needs review', investigating: 'Investigating', accepted: 'Accepted', dismissed: 'Dismissed', resolved: 'Resolved', all: 'All',
}

const statusLabels: Record<string, string> = { ...statuses, open: 'Open' }

function statusTone(s: string) {
  if (s === 'open') return 'warn' as const
  if (s === 'investigating') return 'accent' as const
  return s === 'resolved' || s === 'accepted' ? 'ok' as const : 'neutral' as const
}

const chip = (on: boolean) => `flex h-7 cursor-pointer items-center gap-1.5 rounded-[5px] border px-2 text-[12.5px] ${on ? 'border-accent bg-accent-soft text-accent' : 'border-line bg-surface text-ink-2 hover:border-line-strong'}`

function Chips({ counts, total, active, onPick }: { counts: FindingCount[]; total: number; active: string; onPick: (classification: string) => void }) {
  return (
    <div className="mb-3 flex flex-wrap gap-1.5">
      <button type="button" aria-pressed={active === ''} className={chip(active === '')} onClick={() => onPick('')}>All <span className="font-semibold tabular-nums">{total}</span></button>
      {counts.map((s) => (
        <button key={s.classification} type="button" aria-pressed={active === s.classification} className={chip(active === s.classification)}
          onClick={() => onPick(active === s.classification ? '' : s.classification)}>
          {classifications[s.classification] ?? s.classification}
          <span className="font-semibold tabular-nums">{s.count}</span>
        </button>
      ))}
    </div>
  )
}

function FindingRow({ f, onOpen }: { f: Finding; onOpen: () => void }) {
  return (
    <tr className="cursor-pointer hover:bg-surface-2" onClick={onOpen}>
      <Td className="whitespace-nowrap"><button type="button" className="cursor-pointer font-medium text-ink hover:text-accent">{classifications[f.classification] ?? f.classification}</button></Td>
      <Td className="whitespace-nowrap">{f.address ? <Mono>{f.address}</Mono> : <span className="text-ink-3">—</span>}</Td>
      <Td className="max-w-md text-ink-2"><span className="line-clamp-2">{f.explanation}</span></Td>
      <Td className="whitespace-nowrap text-ink-2">{f.routingDomainName ?? '—'}</Td>
      <Td className="text-right text-ink-2 tabular-nums">{f.confidence === null ? '—' : `${f.confidence}%`}</Td>
      <Td className="whitespace-nowrap text-ink-2"><span title={formatDateTime(f.lastDetectedAt)}>{formatAgo(f.lastDetectedAt)}</span></Td>
      <Td><Badge tone={statusTone(f.reviewStatus)}>{statusLabels[f.reviewStatus] ?? f.reviewStatus}</Badge></Td>
    </tr>
  )
}

export function Findings() {
  const [status, setStatus] = useState('active')
  const [classification, setClassification] = useState('')
  const paging = usePaging()
  const [selected, setSelected] = useState<Finding | null>(null)
  const [search, setSearch] = useState('')
  const q = useDebounced(search.trim())
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, status, classification, q })
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['findings', qs], placeholderData: keepPreviousData, queryFn: () => list<Finding>(`/reconciliation/results${qs}`),
  })
  useClampPage(paging, data?.meta)
  const summary = useQuery({ queryKey: ['findings', 'summary'], queryFn: () => get<FindingCount[]>('/reconciliation/summary') })
  const needsReview = summary.data?.reduce((sum, s) => sum + s.count, 0)
  const filtered = status !== 'active' || classification !== '' || search.trim() !== ''
  const clear = () => { setStatus('active'); setClassification(''); setSearch(''); paging.setPage(1) }
  return (
    <>
      <RunPanel needsReview={needsReview} />
      {summary.data && summary.data.length > 0 && (
        <Chips counts={summary.data} total={needsReview ?? 0} active={classification} onPick={(c) => { setClassification(c); paging.setPage(1) }} />
      )}
      <Card>
        <div className="flex flex-wrap items-center gap-2 border-b border-line p-2.5">
          <Select aria-label="Review status" className="w-44!" value={status} onChange={(e) => { setStatus(e.target.value); paging.setPage(1) }}>
            {Object.entries(statuses).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </Select>
          <Input aria-label="Search findings" className="max-w-xs" placeholder="Search address or detail" value={search} onChange={(e) => { setSearch(e.target.value); paging.setPage(1) }} />
          {filtered && <Button variant="ghost" className="h-8" onClick={clear}>Clear filters</Button>}
        </div>
        {isPending && <SkeletonRows rows={8} />}
        {error && <ErrorState error={error} onRetry={refetch} />}
        {data?.data.length === 0 && (filtered
          ? <EmptyState title="No findings match these filters" action={<Button onClick={clear}>Clear filters</Button>} />
          : <EmptyState title="Nothing needs review" body="Inventory and evidence agree. New differences appear here after the next run." />)}
        {data && data.data.length > 0 && (
          <>
            <div className="overflow-x-auto">
              <table className="w-full border-collapse">
                <thead><tr><Th>Finding</Th><Th>Address</Th><Th>Detail</Th><Th>Routing domain</Th><Th className="text-right">Confidence</Th><Th>Last detected</Th><Th>Status</Th></tr></thead>
                <tbody>{data.data.map((f) => <FindingRow key={f.id} f={f} onOpen={() => setSelected(f)} />)}</tbody>
              </table>
            </div>
            <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
          </>
        )}
      </Card>
      <FindingModal key={selected?.id ?? 'none'} finding={selected} onClose={() => setSelected(null)} />
    </>
  )
}
