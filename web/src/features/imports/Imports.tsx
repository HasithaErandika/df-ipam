import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Download, Upload } from 'lucide-react'
import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import { Badge, Button, Card, EmptyState, ErrorState, PageHeader, Pagination, SkeletonRows, Td, Th } from '../../components/ui'
import { RouteTabs } from '../../components/workflow'
import { list, queryString } from '../../lib/api'
import { formatDateTime } from '../../lib/format'
import { useClampPage, usePaging } from '../../lib/paging'
import { useCan } from '../../lib/session'
import { useRouteTab } from '../../lib/tabs'
import { BatchModal } from './BatchModal'
import { EnrichModal } from './EnrichModal'
import { Records } from './Records'
import { batchLabel, batchTone, formatLabel, skippedCount, templateFormat, useTemplateDownload } from './shared'
import type { Batch, ImportRecord } from './shared'
import { Templates } from './Templates'
import { UploadModal } from './UploadModal'

function BatchRow({ batch, onOpen }: { batch: Batch; onOpen: () => void }) {
  const s = batch.summary
  return (
    <tr className="cursor-pointer hover:bg-surface-2" onClick={onOpen}>
      <Td className="font-medium text-ink"><button type="button" className="cursor-pointer text-left font-medium hover:underline">{batch.fileName}</button><span className="block text-[11.5px] font-normal text-ink-3">{formatLabel(batch.format)}</span></Td>
      <Td className="text-ink-2">{batch.format === templateFormat ? <span className="text-ink-3">Set per row</span> : batch.routingDomainName ?? '—'}</Td>
      <Td className="text-right tabular-nums">{s.lines}</Td>
      <Td className="text-right tabular-nums">{s.valid + s.committed}</Td>
      <Td className="text-right tabular-nums">{skippedCount(s)}</Td>
      <Td className="whitespace-nowrap text-ink-2">{formatDateTime(batch.createdAt)}<span className="block text-[11.5px] text-ink-3">{batch.uploadedByEmail}</span></Td>
      <Td><Badge tone={batchTone[batch.status]}>{batchLabel(batch.status)}</Badge></Td>
    </tr>
  )
}

export function Imports() {
  const canRun = useCan('imports:run')
  const navigate = useNavigate()
  const [params, setParams] = useSearchParams()
  const paging = usePaging()
  const template = useTemplateDownload()
  const [enriching, setEnriching] = useState<ImportRecord | null>(null)
  const tab = useRouteTab(canRun ? ['batches', 'templates', 'queue'] : ['batches'])
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize })
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['imports', 'batches', qs], placeholderData: keepPreviousData, queryFn: () => list<Batch>(`/imports${qs}`) })
  useClampPage(paging, data?.meta)
  const openId = params.get('id')
  const uploading = params.has('upload')
  const setParam = (key: string, value: string | null) => setParams((prev) => { const n = new URLSearchParams(prev); if (value) n.set(key, value); else n.delete(key); return n })
  const closeBatch = () => setParams((prev) => { const n = new URLSearchParams(prev); for (const k of ['id', 'linesPage', 'linesPageSize']) n.delete(k); return n })
  const saveButton = <Button busy={template.isPending} onClick={() => template.mutate('workbook')}><Download size={15} aria-hidden /> Download template</Button>
  const uploadButton = <Button variant="primary" onClick={() => setParam('upload', '1')}><Upload size={15} aria-hidden /> Upload file</Button>
  return (
    <>
      <PageHeader title="Imports" description="Load records from a template or a production export. Every row is kept, checked and shown; a second person approves; a committed import can be reversed while untouched."
        actions={canRun && <>{tab !== 'templates' && saveButton}{uploadButton}</>} />
      {canRun && <RouteTabs tabs={[{ key: 'batches', label: 'Imports' }, { key: 'templates', label: 'Templates' }, { key: 'queue', label: 'Enrichment queue' }]} active={tab} base="/imports" />}
      {tab === 'batches' && (
        <Card>
          {isPending && <SkeletonRows rows={5} />}
          {error && <ErrorState error={error} onRetry={refetch} />}
          {data?.data.length === 0 && <EmptyState title="No imports yet" body={canRun ? 'Download the template, fill it in and upload it. Production exports can be uploaded too.' : 'Imports appear here once someone uploads a file.'}
            action={canRun && <div className="flex gap-2">{saveButton}{uploadButton}</div>} />}
          {data && data.data.length > 0 && (
            <>
              <div className="overflow-x-auto">
                <table className="w-full border-collapse">
                  <thead><tr><Th>File</Th><Th>Routing domain</Th><Th className="text-right">Rows</Th><Th className="text-right">Ready or imported</Th><Th className="text-right">Skipped</Th><Th>Uploaded</Th><Th>Status</Th></tr></thead>
                  <tbody>{data.data.map((b) => <BatchRow key={b.id} batch={b} onOpen={() => setParam('id', b.id)} />)}</tbody>
                </table>
              </div>
              <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
            </>
          )}
        </Card>
      )}
      {tab === 'templates' && <Templates />}
      {tab === 'queue' && <Card><div className="p-3"><Records path="/imports/enrichment-queue" onEnrich={setEnriching} /></div></Card>}
      {uploading && canRun && <UploadModal onClose={() => setParam('upload', null)} onDone={(id) => navigate(`/imports/batches?id=${id}`)} />}
      {openId && <BatchModal key={openId} id={openId} onClose={closeBatch} />}
      {enriching && <EnrichModal record={enriching} onClose={() => setEnriching(null)} />}
    </>
  )
}
