import { useRouteTab } from '../../lib/tabs'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Upload } from 'lucide-react'
import { useState } from 'react'
import { useSearchParams } from 'react-router'
import { Badge, Button, Card, EmptyState, ErrorState, PageHeader, Pagination, SkeletonRows, Td, Th } from '../../components/ui'
import { RouteTabs } from '../../components/workflow'
import { BatchModal, EnrichModal, Records } from './BatchModal'
import { batchTone, formats } from './shared'
import type { Batch, ImportRecord } from './shared'
import { UploadModal } from './UploadModal'
import { list, queryString } from '../../lib/api'
import { formatDateTime } from '../../lib/format'
import { useClampPage, usePaging } from '../../lib/paging'
import { useCan } from '../../lib/session'

export function Imports() {
  const canRun = useCan('imports:run')
  const [params, setParams] = useSearchParams()
  const paging = usePaging()
  const [uploading, setUploading] = useState(false)
  const [enriching, setEnriching] = useState<ImportRecord | null>(null)
  const tab = useRouteTab(['batches', 'queue'])
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize })
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['imports', 'batches', qs], placeholderData: keepPreviousData, queryFn: () => list<Batch>(`/imports${qs}`) })
  useClampPage(paging, data?.meta)
  const openId = params.get('id')
  const open = data?.data.find((b) => b.id === openId) ?? null
  const setOpen = (id: string | null) => setParams((prev) => { const n = new URLSearchParams(prev); if (id) n.set('id', id); else n.delete('id'); return n })
  return (
    <>
      <PageHeader title="Imports" description="Load production exports. Every source line is kept, checked and shown; a second person approves; a committed import can be reversed while untouched."
        actions={canRun && <Button variant="primary" onClick={() => setUploading(true)}><Upload size={15} aria-hidden /> Upload export</Button>} />
      <RouteTabs tabs={[{ key: 'batches', label: 'Imports' }, { key: 'queue', label: 'Enrichment queue' }]} active={tab} base="/imports" />
      {tab === 'batches' && (
        <Card>
          {isPending && <SkeletonRows rows={5} />}
          {error && <ErrorState error={error} onRetry={refetch} />}
          {data?.data.length === 0 && <EmptyState title="No imports yet" body="The production files Prod_Subnets.txt and Prod_VDOMs_Subnets.txt haven't arrived; the parser accepts CIDRs, address and mask pairs, and VDOM-prefixed lines in the meantime." />}
          {data && data.data.length > 0 && (
            <>
              <table className="w-full border-collapse">
                <thead><tr><Th>File</Th><Th>Routing domain</Th><Th className="text-right">Lines</Th><Th className="text-right">Valid</Th><Th className="text-right">Problems</Th><Th>Uploaded</Th><Th>Status</Th></tr></thead>
                <tbody>{data.data.map((b) => (
                  <tr key={b.id} className="cursor-pointer hover:bg-surface-2" onClick={() => setOpen(b.id)}>
                    <Td className="font-medium text-ink">{b.fileName}<span className="block text-[11.5px] font-normal text-ink-3">{formats[b.format]}</span></Td>
                    <Td className="text-ink-2">{b.routingDomainName ?? '—'}</Td>
                    <Td className="text-right tabular-nums">{b.summary.lines}</Td>
                    <Td className="text-right tabular-nums">{b.summary.valid + b.summary.committed}</Td>
                    <Td className="text-right tabular-nums">{b.summary.invalid + b.summary.duplicate + b.summary.overlap + b.summary.needsEnrichment}</Td>
                    <Td className="whitespace-nowrap text-ink-2">{formatDateTime(b.createdAt)}<span className="block text-[11.5px] text-ink-3">{b.uploadedByEmail}</span></Td>
                    <Td><Badge tone={batchTone[b.status]}>{b.status === 'validated' ? 'Waiting for approval' : b.status}</Badge></Td>
                  </tr>
                ))}</tbody>
              </table>
              <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
            </>
          )}
        </Card>
      )}
      {tab === 'queue' && <Card><div className="p-3"><Records path="/imports/enrichment-queue" showFile canEnrich={canRun} onEnrich={setEnriching} /></div></Card>}
      {uploading && <UploadModal onClose={() => setUploading(false)} onDone={(id) => { setParams({ tab: 'batches', id }) }} />}
      {open && <BatchModal key={open.id} batch={open} onClose={() => setOpen(null)} />}
      {enriching && <EnrichModal record={enriching} onClose={() => setEnriching(null)} />}
    </>
  )
}
