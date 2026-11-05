import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Badge, Card, Field, Input, Modal, MoreOptions, EmptyState, ErrorState, Mono, PageHeader, Pagination, Select, SkeletonRows, Td, Th } from '../components/ui'
import { get, list, queryString } from '../lib/api'
import { SearchSelect } from '../components/SearchSelect'
import { useDebounced } from '../components/workflow'
import { dateOrder } from '../lib/validation'
import { formatDateTime, humanAction } from '../lib/format'
import { useClampPage, usePaging } from '../lib/paging'
import { useCan } from '../lib/session'
import type { AuditEntry } from '../lib/types'

function Values({ title, value }: { title: string; value: unknown }) {
  if (value === null || value === undefined) return null
  return (
    <>
      <h3 className="mt-4 mb-1 text-[13px] font-semibold text-ink">{title}</h3>
      <pre className="overflow-x-auto rounded-md border border-line bg-surface-2 p-3 font-mono text-xs text-ink-2">{JSON.stringify(value, null, 2)}</pre>
    </>
  )
}

export function Audit() {
  const canView = useCan('audit:view')
  const paging = usePaging()
  const [selected, setSelected] = useState<AuditEntry | null>(null)
  const [filters, setFilters] = useState({ actor: '', action: '', objectType: '', result: '', from: '', to: '' })
  const setFilter = (key: keyof typeof filters, value: string) => { setFilters((f) => ({ ...f, [key]: value })); paging.setPage(1) }
  const actor = useDebounced(filters.actor.trim())
  const badDates = dateOrder(filters.from, filters.to)
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, ...filters, actor, ...(badDates ? { from: '', to: '' } : {}) })
  const facets = useQuery({ queryKey: ['audit', 'filters'], enabled: canView, queryFn: () => get<{ actions: string[]; objectTypes: string[] }>('/audit-logs/filters') })
  const facetProps = { loading: facets.isPending, fetchError: facets.error, onRetry: () => facets.refetch() }
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['audit', qs], enabled: canView, placeholderData: keepPreviousData,
    queryFn: () => list<AuditEntry>(`/audit-logs${qs}`),
  })
  useClampPage(paging, data?.meta)
  return (
    <>
      <PageHeader title="Audit Logs" description="An append-only record of every important change." />
      <Card>
        {canView && (
          <div className="space-y-2 border-b border-line p-2.5">
            <div className="grid gap-2 sm:grid-cols-3">
            <Field label="Actor email" htmlFor="audit-actor"><Input id="audit-actor" value={filters.actor} onChange={(e) => setFilter('actor', e.target.value)} /></Field>
            <Field label="From" htmlFor="audit-from"><Input id="audit-from" type="date" value={filters.from} onChange={(e) => setFilter('from', e.target.value)} /></Field>
            <Field label="To" htmlFor="audit-to" error={badDates}><Input id="audit-to" type="date" min={filters.from} value={filters.to} onChange={(e) => setFilter('to', e.target.value)} /></Field>
            </div>
            <MoreOptions label="More filters">
              <div className="grid gap-2 sm:grid-cols-3">
            <Field label="Action" htmlFor="audit-action"><SearchSelect id="audit-action" emptyLabel="Any" value={filters.action} onChange={(v) => setFilter('action', v)} {...facetProps}
              options={facets.data?.actions.map((a) => ({ value: a, label: humanAction(a), detail: a })) ?? []} /></Field>
            <Field label="Object type" htmlFor="audit-object"><SearchSelect id="audit-object" emptyLabel="Any" value={filters.objectType} onChange={(v) => setFilter('objectType', v)} {...facetProps}
              options={facets.data?.objectTypes.map((o) => ({ value: o, label: o.replaceAll('_', ' ') })) ?? []} /></Field>
            <Field label="Result" htmlFor="audit-result">
              <Select id="audit-result" value={filters.result} onChange={(e) => setFilter('result', e.target.value)}>
                <option value="">Any</option><option value="success">Success</option><option value="failure">Failure</option>
              </Select>
            </Field>
              </div>
            </MoreOptions>
          </div>
        )}
        {!canView && <EmptyState title="You don't have access to audit history" />}
        {canView && isPending && <SkeletonRows rows={8} />}
        {error && <ErrorState error={error} onRetry={refetch} />}
        {data?.data.length === 0 && <EmptyState title="No changes match" />}
        {data && data.data.length > 0 && (
          <>
            <div className="overflow-x-auto">
              <table className="w-full border-collapse">
                <thead><tr><Th>Time</Th><Th>Actor</Th><Th>Action</Th><Th>Object</Th><Th>Source IP</Th><Th>Result</Th></tr></thead>
                <tbody>
                  {data.data.map((e) => (
                    <tr key={e.id} className="hover:bg-surface-2">
                      <Td className="whitespace-nowrap text-ink-2">{formatDateTime(e.occurredAt)}</Td>
                      <Td className="text-ink-2">{e.actorEmail ?? e.actorType}</Td>
                      <Td><button type="button" className="cursor-pointer font-medium text-ink hover:text-accent" onClick={() => setSelected(e)}>{humanAction(e.action)}</button></Td>
                      <Td className="text-ink-2">{e.objectType.replaceAll('_', ' ')}</Td>
                      <Td className="text-ink-2">{e.sourceIp ? <Mono>{e.sourceIp}</Mono> : '—'}</Td>
                      <Td><Badge tone={e.result === 'success' ? 'ok' : 'danger'}>{e.result}</Badge></Td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
          </>
        )}
      </Card>
      <Modal open={selected !== null} onClose={() => setSelected(null)} title={selected ? humanAction(selected.action) : ''}
        subtitle={selected && `${selected.actorEmail ?? selected.actorType} · ${formatDateTime(selected.occurredAt)}`}>
        {selected?.correlationId && <p className="text-[13px] text-ink-2">Correlation ID: <Mono>{selected.correlationId}</Mono></p>}
        <Values title="Previous values" value={selected?.oldValues} />
        <Values title="New values" value={selected?.newValues} />
      </Modal>
    </>
  )
}
