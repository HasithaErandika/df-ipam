import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Badge, Button, EmptyState, ErrorState, Mono, Pagination, Select, SkeletonRows, Td, Th } from '../../components/ui'
import { list, queryString } from '../../lib/api'
import { useClampPage, usePaging } from '../../lib/paging'
import { recordLabel, recordTone, statusOptions, templateFormat } from './shared'
import type { Batch, ImportRecord } from './shared'

const correctable = ['invalid', 'needs_enrichment', 'overlap', 'duplicate', 'valid']

function Problems({ errors }: { errors: string[] }) {
  if (errors.length === 0) return null
  return <ul className="mt-1 space-y-0.5 text-[12px] text-ink-2">{errors.map((e) => <li key={e}>{e}</li>)}</ul>
}

function StatusCell({ record, template }: { record: ImportRecord; template: boolean }) {
  if (record.parsed.comment) return <Badge tone="neutral">Ignored</Badge>
  return <><Badge tone={recordTone[record.status] ?? 'neutral'}>{recordLabel(record.status, template)}</Badge><Problems errors={record.errors} /></>
}

function TemplateRows({ rows }: { rows: ImportRecord[] }) {
  return (
    <table className="w-full border-collapse">
      <thead><tr><Th>Sheet</Th><Th className="text-right">Row</Th><Th>Record</Th><Th>Values</Th><Th>Status</Th></tr></thead>
      <tbody>{rows.map((r) => (
        <tr key={r.id}>
          <Td className="whitespace-nowrap text-ink-2">{r.parsed.sheet}</Td>
          <Td className="text-right tabular-nums text-ink-3">{r.parsed.row}</Td>
          <Td className="font-medium text-ink">{r.parsed.name || '—'}</Td>
          <Td className="max-w-64 truncate font-mono text-[12px] text-ink-2"><span title={r.rawText}>{r.rawText}</span></Td>
          <Td className="min-w-56"><StatusCell record={r} template /></Td>
        </tr>
      ))}</tbody>
    </table>
  )
}

function ExportRows({ rows, showFile, onEnrich }: { rows: ImportRecord[]; showFile?: boolean; onEnrich?: (r: ImportRecord) => void }) {
  return (
    <table className="w-full border-collapse">
      <thead><tr>{showFile && <Th>File</Th>}<Th>Line</Th><Th>Source text</Th><Th>Network</Th><Th>Name</Th><Th>Status</Th>{onEnrich && <Th />}</tr></thead>
      <tbody>{rows.map((r) => (
        <tr key={r.id}>
          {showFile && <Td className="text-ink-2">{r.fileName}</Td>}
          <Td className="tabular-nums text-ink-3">{r.lineNumber}</Td>
          <Td className="max-w-72 truncate font-mono text-[12px] text-ink-2"><span title={r.rawText}>{r.rawText || <span className="text-ink-3">(blank)</span>}</span></Td>
          <Td>{r.parsed.cidr ? <Mono>{r.parsed.cidr}</Mono> : '—'}{r.parsed.vdom && <span className="ml-1 text-[12px] text-ink-3">VDOM {r.parsed.vdom}</span>}</Td>
          <Td className="text-ink-2">{r.parsed.name || '—'}</Td>
          <Td><StatusCell record={r} template={false} /></Td>
          {onEnrich && <Td className="text-right">{correctable.includes(r.status) && !r.parsed.comment && <Button variant="ghost" className="h-7" onClick={() => onEnrich(r)}>Correct</Button>}</Td>}
        </tr>
      ))}</tbody>
    </table>
  )
}

/** A batch's lines, or with no batch the enrichment queue across all of them. */
export function Records({ path, batch, onEnrich }: { path: string; batch?: Batch; onEnrich?: (r: ImportRecord) => void }) {
  const paging = usePaging('lines', 100)
  const [status, setStatus] = useState('')
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, status })
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['imports', path, qs], placeholderData: keepPreviousData, queryFn: () => list<ImportRecord>(`${path}${qs}`) })
  useClampPage(paging, data?.meta)
  const template = batch?.format === templateFormat
  return (
    <>
      {batch && (
        <div className="mb-2"><Select aria-label="Filter by status" className="w-56!" value={status} onChange={(e) => { setStatus(e.target.value); paging.setPage(1) }}>
          <option value="">All {template ? 'rows' : 'lines'} ({batch.summary.lines})</option>{statusOptions(batch).map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </Select></div>
      )}
      {isPending && <SkeletonRows rows={6} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && (batch ? <EmptyState title="Nothing with this status" /> : <EmptyState title="Nothing waiting" body="Lines from production exports that need a person to finish them appear here." />)}
      {data && data.data.length > 0 && (
        <div className="overflow-x-auto rounded-[5px] border border-line">
          {template ? <TemplateRows rows={data.data} /> : <ExportRows rows={data.data} showFile={!batch} onEnrich={onEnrich} />}
          <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
        </div>
      )}
    </>
  )
}
