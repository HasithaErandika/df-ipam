import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Badge, EmptyState, ErrorState, Modal, Mono, Pagination, SkeletonRows, Td, Th } from '../../components/ui'
import { list } from '../../lib/api'
import { formatDateTime } from '../../lib/format'
import { usePaging } from '../../lib/paging'
import type { Connection, SyncError, SyncJob } from '../../lib/types'
import { jobTone } from './shared'

function SyncErrors({ job }: { job: SyncJob }) {
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['sync-errors', job.id], queryFn: () => list<SyncError>(`/integrations/syncs/${job.id}/errors?pageSize=50`) })
  if (isPending) return <SkeletonRows rows={2} />
  if (error) return <ErrorState error={error} onRetry={refetch} />
  return <ul className="space-y-1 p-2">{data.data.map((e, i) => <li key={i} className="text-[13px] text-ink-2"><Mono>{e.code}</Mono> {e.message}</li>)}</ul>
}

export function History({ connection, onClose }: { connection: Connection | null; onClose: () => void }) {
  const paging = usePaging('hp')
  const [open, setOpen] = useState<string | null>(null)
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['syncs', connection?.id, paging.page, paging.pageSize], enabled: connection !== null, placeholderData: keepPreviousData,
    queryFn: () => list<SyncJob>(`/integrations/connections/${connection?.id}/syncs?page=${paging.page}&pageSize=${paging.pageSize}`),
    refetchInterval: (q) => (q.state.data?.data.some((j) => j.status === 'queued' || j.status === 'running') ? 5000 : false),
  })
  if (!connection) return null
  return (
    <Modal open onClose={onClose} title="Sync history" subtitle={connection.name} size="lg">
      {isPending && <SkeletonRows rows={5} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && <EmptyState title="No syncs yet" />}
      {data && data.data.length > 0 && (
        <>
          <table className="w-full border-collapse">
            <thead><tr><Th>Started</Th><Th>Status</Th><Th>Stored</Th><Th>Problems</Th></tr></thead>
            <tbody>
              {data.data.map((j) => (
                <tr key={j.id}>
                  <Td className="whitespace-nowrap text-ink-2">{formatDateTime(j.startedAt ?? j.createdAt)}</Td>
                  <Td><Badge tone={jobTone(j.status)}>{j.status}</Badge></Td>
                  <Td className="text-ink-2">{j.status === 'completed' ? `${j.counts.devices ?? 0} devices, ${j.counts.observations ?? 0} observations${j.counts.unmappedObservations ? `, ${j.counts.unmappedObservations} not mapped` : ''}` : j.error}</Td>
                  <Td>
                    {j.errorCount > 0 ? <button type="button" className="cursor-pointer text-accent hover:underline" onClick={() => setOpen(open === j.id ? null : j.id)}>{j.errorCount}</button> : '—'}
                    {open === j.id && <SyncErrors job={j} />}
                  </Td>
                </tr>
              ))}
            </tbody>
          </table>
          <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
        </>
      )}
    </Modal>
  )
}
