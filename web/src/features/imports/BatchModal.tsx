import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { Undo2 } from 'lucide-react'
import { useState } from 'react'
import { Badge, Button, ConfirmDialog, EmptyState, ErrorState, Field, FormError, Input, Modal, Mono, Pagination, Select, SkeletonRows, Td, Th, useConfirmSave } from '../../components/ui'
import { Detail } from '../../components/workflow'
import { useChecks } from '../../components/workflow'
import { list, patchPlain, post, queryString } from '../../lib/api'
import { environments, formatDateTime } from '../../lib/format'
import { useClampPage, usePaging } from '../../lib/paging'
import { useCan, useMe } from '../../lib/session'
import { between, cidr as cidrIssue, issues as check, optionalIP } from '../../lib/validation'
import { batchTone, formats, recordTone, useRefresh } from './shared'
import type { Batch, ImportRecord } from './shared'

export function EnrichModal({ record, onClose }: { record: ImportRecord; onClose: () => void }) {
  const refresh = useRefresh()
  const [form, setForm] = useState({ cidr: record.parsed.cidr ?? '', name: record.parsed.name ?? '', gateway: record.parsed.gateway ?? '', vdom: record.parsed.vdom ?? '', vlan: record.parsed.vlan ?? '' })
  const confirmSave = useConfirmSave()
  const save = useMutation({ meta: { success: 'Line corrected.', inlineError: true }, mutationFn: () => patchPlain<ImportRecord>(`/imports/records/${record.id}`, form), onSuccess: () => { refresh(); onClose() } })
  const set = (k: keyof typeof form) => (e: { target: { value: string } }) => setForm({ ...form, [k]: e.target.value })
  const checks = useChecks(check({ cidr: form.cidr.trim() ? cidrIssue(form.cidr) : undefined, gateway: optionalIP(form.gateway), vlan: form.vlan.trim() ? between(form.vlan, 1, 4094, 'VLAN') : undefined }))
  return (
    <Modal open onClose={onClose} title={`Correct line ${record.lineNumber}`} subtitle={<Mono>{record.rawText}</Mono>}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => checks.submit(() => confirmSave(`line ${record.lineNumber}`, () => save.mutate()))}>Save and recheck</Button></>}>
      <div className="grid grid-cols-2 gap-3">
        <Field label="CIDR" htmlFor="en-cidr" error={checks.shown.cidr}><Input id="en-cidr" className="font-mono" value={form.cidr} onChange={set('cidr')} /></Field>
        <Field label="Name" htmlFor="en-name"><Input id="en-name" value={form.name} onChange={set('name')} /></Field>
        <Field label="Gateway" htmlFor="en-gw" error={checks.shown.gateway}><Input id="en-gw" className="font-mono" value={form.gateway} onChange={set('gateway')} /></Field>
        <Field label="VDOM" htmlFor="en-vdom"><Input id="en-vdom" value={form.vdom} onChange={set('vdom')} /></Field>
        <Field label="VLAN" htmlFor="en-vlan" error={checks.shown.vlan}><Input id="en-vlan" value={form.vlan} onChange={set('vlan')} /></Field>
      </div>
      <p className="mt-2 text-[12.5px] text-ink-3">The original line is kept unchanged for the record.</p>
      <div className="mt-2"><FormError error={save.error} /></div>
    </Modal>
  )
}

export function Records({ path, showFile, onEnrich, canEnrich }: { path: string; showFile?: boolean; onEnrich: (r: ImportRecord) => void; canEnrich: boolean }) {
  const paging = usePaging('lines', 100)
  const [status, setStatus] = useState('')
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, status })
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['imports', path, qs], placeholderData: keepPreviousData, queryFn: () => list<ImportRecord>(`${path}${qs}`) })
  useClampPage(paging, data?.meta)
  return (
    <>
      {!showFile && (
        <div className="mb-2"><Select aria-label="Line status" className="w-48!" value={status} onChange={(e) => { setStatus(e.target.value); paging.setPage(1) }}>
          <option value="">All lines</option>{Object.keys(recordTone).map((s) => <option key={s} value={s}>{s.replace('_', ' ')}</option>)}
        </Select></div>
      )}
      {isPending && <SkeletonRows rows={6} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && <EmptyState title="No lines here" />}
      {data && data.data.length > 0 && (
        <div className="overflow-x-auto rounded-[5px] border border-line">
          <table className="w-full border-collapse">
            <thead><tr>{showFile && <Th>File</Th>}<Th>Line</Th><Th>Source text</Th><Th>Network</Th><Th>Name</Th><Th>Status</Th>{canEnrich && <Th />}</tr></thead>
            <tbody>{data.data.map((r) => (
              <tr key={r.id}>
                {showFile && <Td className="text-ink-2">{r.fileName}</Td>}
                <Td className="tabular-nums text-ink-3">{r.lineNumber}</Td>
                <Td className="max-w-72 truncate font-mono text-[12px] text-ink-2" >{r.rawText || <span className="text-ink-3">(blank)</span>}</Td>
                <Td>{r.parsed.cidr ? <Mono>{r.parsed.cidr}</Mono> : '—'}{r.parsed.vdom && <span className="ml-1 text-[12px] text-ink-3">VDOM {r.parsed.vdom}</span>}</Td>
                <Td className="text-ink-2">{r.parsed.name || '—'}</Td>
                <Td><Badge tone={recordTone[r.status] ?? 'neutral'}>{r.parsed.comment ? 'ignored' : r.status.replace('_', ' ')}</Badge>
                  {r.errors.length > 0 && !r.parsed.comment && <span className="block text-[12px] text-ink-3">{r.errors[0]}</span>}</Td>
                {canEnrich && <Td className="text-right">{['invalid', 'needs_enrichment', 'overlap', 'duplicate', 'valid'].includes(r.status) && !r.parsed.comment &&
                  <Button variant="ghost" className="h-7" onClick={() => onEnrich(r)}>Correct</Button>}</Td>}
              </tr>
            ))}</tbody>
          </table>
          <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
        </div>
      )}
    </>
  )
}

export function BatchModal({ batch, onClose }: { batch: Batch; onClose: () => void }) {
  const refresh = useRefresh()
  const me = useMe()
  const canRun = useCan('imports:run')
  const [confirm, setConfirm] = useState<'approve' | 'reverse' | null>(null)
  const [enriching, setEnriching] = useState<ImportRecord | null>(null)
  const act = useMutation({ meta: { success: (a: string) => a === 'approve' ? 'Import approved and committed.' : 'Import reversed.', inlineError: true }, mutationFn: (a: 'approve' | 'reverse') => post<Batch>(`/imports/${batch.id}/${a}`), onSuccess: () => { refresh(); setConfirm(null); onClose() } })
  const s = batch.summary
  const own = batch.uploadedBy === me.id
  return (
    <>
      <Modal open onClose={onClose} size="xl" title={batch.fileName} subtitle={<span className="flex items-center gap-2"><Badge tone={batchTone[batch.status]}>{batch.status}</Badge>{formats[batch.format]}</span>}
        footer={canRun && <>
          {batch.status === 'validated' && (own ? <span className="mr-auto text-[12.5px] text-ink-3">Someone else must approve your upload.</span>
            : <Button variant="primary" disabled={s.valid === 0} onClick={() => { act.reset(); setConfirm('approve') }}>Import {s.valid} subnets</Button>)}
          {batch.status === 'committed' && <Button onClick={() => { act.reset(); setConfirm('reverse') }}><Undo2 size={14} aria-hidden /> Reverse import</Button>}
        </>}>
        <div className="mb-3 grid grid-cols-2 gap-x-6 md:grid-cols-4">
          <dl><Detail label="Lines">{s.lines}</Detail><Detail label="Valid">{s.valid}</Detail></dl>
          <dl><Detail label="Needs enrichment">{s.needsEnrichment}</Detail><Detail label="Invalid">{s.invalid}</Detail></dl>
          <dl><Detail label="Duplicate">{s.duplicate}</Detail><Detail label="Overlap">{s.overlap}</Detail></dl>
          <dl><Detail label="Ignored (blank, comment)">{s.ignored}</Detail><Detail label="Imported">{s.committed}</Detail></dl>
        </div>
        <p className="mb-3 text-[12.5px] text-ink-3">{batch.routingDomainName} · {batch.environment && environments[batch.environment]} · uploaded by {batch.uploadedByEmail} {formatDateTime(batch.createdAt)}
          {batch.approvedByEmail && ` · approved by ${batch.approvedByEmail}`} · SHA-256 <Mono>{batch.fileHash.slice(0, 12)}…</Mono></p>
        <Records path={`/imports/${batch.id}/preview`} onEnrich={setEnriching} canEnrich={canRun && batch.status === 'validated'} />
      </Modal>
      {enriching && <EnrichModal record={enriching} onClose={() => setEnriching(null)} />}
      <ConfirmDialog open={confirm !== null} title={confirm === 'approve' ? `Import ${s.valid} subnets?` : 'Reverse this import?'}
        body={confirm === 'approve' ? 'All valid lines become subnets together, or none do. Other lines stay recorded and are not imported.'
          : 'The subnets this batch created are removed, if none has gained IP records or links since. Otherwise nothing changes.'}
        confirmLabel={confirm === 'approve' ? 'Import' : 'Reverse'} busy={act.isPending} error={act.error}
        onConfirm={() => confirm && act.mutate(confirm)} onCancel={() => setConfirm(null)} />
    </>
  )
}
