import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Badge, Card, Modal, EmptyState, ErrorState, Mono, PageHeader, Pagination, SkeletonRows, Td, Th } from '../components/ui'
import { list } from '../lib/api'
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
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['audit', paging.page, paging.pageSize], enabled: canView, placeholderData: keepPreviousData,
    queryFn: () => list<AuditEntry>(`/audit-logs?page=${paging.page}&pageSize=${paging.pageSize}`),
  })
  useClampPage(paging, data?.meta)
  return (
    <>
      <PageHeader title="Audit Logs" description="An append-only record of every important change." />
      <Card>
        {!canView && <EmptyState title="You don't have access to audit history" />}
        {canView && isPending && <SkeletonRows rows={8} />}
        {error && <ErrorState error={error} onRetry={refetch} />}
        {data?.data.length === 0 && <EmptyState title="No changes recorded yet" />}
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
