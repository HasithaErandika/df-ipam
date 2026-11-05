import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Undo2, Upload } from 'lucide-react'
import { useState } from 'react'
import { useSearchParams } from 'react-router'
import { Badge, Button, Card, ConfirmDialog, EmptyState, ErrorState, Field, FormError, Input, Modal, Mono, PageHeader, Pagination, Select, SkeletonRows, Td, Th } from '../components/ui'
import { Detail, Tabs } from '../components/workflow'
import { ApiError, list, listAll, patchPlain, post, queryString } from '../lib/api'
import { environments, formatDateTime } from '../lib/format'
import { useClampPage, usePaging } from '../lib/paging'
import { useCan, useMe } from '../lib/session'
import type { RoutingDomain } from '../lib/types'

interface Summary { lines: number; valid: number; invalid: number; ignored: number; duplicate: number; overlap: number; needsEnrichment: number; committed: number }
interface Batch {
  id: string; fileName: string; fileHash: string; format: string; status: string; summary: Summary; routingDomainName: string | null; environment: string | null
  uploadedBy: string; uploadedByEmail: string; approvedByEmail: string | null; createdAt: string; committedAt: string | null; reversedAt: string | null
}
interface Parsed { cidr?: string; name?: string; gateway?: string; vdom?: string; vlan?: string; comment?: boolean }
interface ImportRecord { id: string; batchId: string; lineNumber: number; rawText: string; parsed: Parsed; status: string; errors: string[]; createdObjectId: string | null; fileName?: string }

const formats: Record<string, string> = {
  prod_subnets: 'Production subnets (Prod_Subnets.txt)', prod_vdoms_subnets: 'Production VDOM subnets (Prod_VDOMs_Subnets.txt)', csv: 'CSV: cidr, name, gateway, vlan',
}
const recordTone: Record<string, 'ok' | 'danger' | 'warn' | 'neutral' | 'accent'> = {
  valid: 'ok', invalid: 'danger', duplicate: 'neutral', overlap: 'warn', needs_enrichment: 'warn', committed: 'accent',
}
const batchTone: Record<string, 'ok' | 'danger' | 'warn' | 'neutral' | 'accent'> = {
  validated: 'warn', committed: 'ok', reversed: 'neutral', failed: 'danger', uploaded: 'neutral', approved: 'accent',
}

function useRefresh() {
  const queryClient = useQueryClient()
  return () => { for (const k of ['imports', 'subnets', 'summary']) queryClient.invalidateQueries({ queryKey: [k] }) }
}

function UploadModal({ onClose, onDone }: { onClose: () => void; onDone: (id: string) => void }) {
  const refresh = useRefresh()
  const domains = useQuery({ queryKey: ['routing-domains', 'all'], queryFn: () => listAll<RoutingDomain>('/routing-domains') })
  const [file, setFile] = useState<File | null>(null)
  const [format, setFormat] = useState('prod_subnets')
  const [routingDomainId, setRd] = useState('')
  const [environment, setEnvironment] = useState('production')
  const upload = useMutation({
    mutationFn: async () => post<Batch>('/imports', { fileName: file?.name ?? '', format, routingDomainId, environment, content: file ? await file.text() : '' }),
    onSuccess: (b) => { refresh(); onClose(); onDone(b.id) },
  })
  const errors: Record<string, string> = {}
  if (upload.error instanceof ApiError) for (const d of upload.error.details) errors[d.field] = d.issue
  return (
    <Modal open onClose={onClose} title="Upload an export" subtitle="Every line is kept and checked. Nothing reaches the inventory until someone else approves the batch."
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={upload.isPending} disabled={!file} onClick={() => upload.mutate()}><Upload size={14} aria-hidden /> Upload and check</Button></>}>
      <div className="space-y-3.5">
        <Field label="File" htmlFor="imp-file" error={errors.content ?? errors.fileName} hint="Plain text, at most 5 MB.">
          <input id="imp-file" type="file" accept=".txt,.csv,text/plain" onChange={(e) => setFile(e.target.files?.[0] ?? null)} className="block text-[13px]" />
        </Field>
        <Field label="Format" htmlFor="imp-format" error={errors.format}>
          <Select id="imp-format" value={format} onChange={(e) => setFormat(e.target.value)}>{Object.entries(formats).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</Select>
        </Field>
        <Field label="Routing domain" htmlFor="imp-rd" error={errors.routingDomainId}>
          <Select id="imp-rd" value={routingDomainId} onChange={(e) => setRd(e.target.value)}>
            <option value="">Select</option>{domains.data?.map((d) => <option key={d.id} value={d.id}>{d.organisationName} / {d.name}</option>)}
          </Select>
        </Field>
        <Field label="Environment" htmlFor="imp-env" error={errors.environment}>
          <Select id="imp-env" value={environment} onChange={(e) => setEnvironment(e.target.value)}>{Object.entries(environments).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</Select>
        </Field>
        {Object.keys(errors).length === 0 && <FormError error={upload.error} />}
      </div>
    </Modal>
  )
}

function EnrichModal({ record, onClose }: { record: ImportRecord; onClose: () => void }) {
  const refresh = useRefresh()
  const [form, setForm] = useState({ cidr: record.parsed.cidr ?? '', name: record.parsed.name ?? '', gateway: record.parsed.gateway ?? '', vdom: record.parsed.vdom ?? '', vlan: record.parsed.vlan ?? '' })
  const save = useMutation({ mutationFn: () => patchPlain<ImportRecord>(`/imports/records/${record.id}`, form), onSuccess: () => { refresh(); onClose() } })
  const set = (k: keyof typeof form) => (e: { target: { value: string } }) => setForm({ ...form, [k]: e.target.value })
  return (
    <Modal open onClose={onClose} title={`Correct line ${record.lineNumber}`} subtitle={<Mono>{record.rawText}</Mono>}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => save.mutate()}>Save and recheck</Button></>}>
      <div className="grid grid-cols-2 gap-3">
        <Field label="CIDR" htmlFor="en-cidr"><Input id="en-cidr" className="font-mono" value={form.cidr} onChange={set('cidr')} /></Field>
        <Field label="Name" htmlFor="en-name"><Input id="en-name" value={form.name} onChange={set('name')} /></Field>
        <Field label="Gateway" htmlFor="en-gw"><Input id="en-gw" className="font-mono" value={form.gateway} onChange={set('gateway')} /></Field>
        <Field label="VDOM" htmlFor="en-vdom"><Input id="en-vdom" value={form.vdom} onChange={set('vdom')} /></Field>
        <Field label="VLAN" htmlFor="en-vlan"><Input id="en-vlan" value={form.vlan} onChange={set('vlan')} /></Field>
      </div>
      <p className="mt-2 text-[12.5px] text-ink-3">The original line is kept unchanged for the record.</p>
      <div className="mt-2"><FormError error={save.error} /></div>
    </Modal>
  )
}

function Records({ path, showFile, onEnrich, canEnrich }: { path: string; showFile?: boolean; onEnrich: (r: ImportRecord) => void; canEnrich: boolean }) {
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

function BatchModal({ batch, onClose }: { batch: Batch; onClose: () => void }) {
  const refresh = useRefresh()
  const me = useMe()
  const canRun = useCan('imports:run')
  const [confirm, setConfirm] = useState<'approve' | 'reverse' | null>(null)
  const [enriching, setEnriching] = useState<ImportRecord | null>(null)
  const act = useMutation({ mutationFn: (a: 'approve' | 'reverse') => post<Batch>(`/imports/${batch.id}/${a}`), onSuccess: () => { refresh(); setConfirm(null); onClose() } })
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

export function Imports() {
  const canRun = useCan('imports:run')
  const [params, setParams] = useSearchParams()
  const paging = usePaging()
  const [uploading, setUploading] = useState(false)
  const [enriching, setEnriching] = useState<ImportRecord | null>(null)
  const tab = params.get('tab') === 'queue' ? 'queue' : 'batches'
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
      <Tabs tabs={[{ key: 'batches', label: 'Imports' }, { key: 'queue', label: 'Enrichment queue' }]} active={tab} onChange={(k) => setParams({ tab: k })} />
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
