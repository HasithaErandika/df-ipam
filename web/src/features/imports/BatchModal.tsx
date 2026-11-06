import { useMutation, useQuery } from '@tanstack/react-query'
import { Trash2, Undo2 } from 'lucide-react'
import { useState } from 'react'
import { Badge, Button, ConfirmDialog, ErrorState, Modal, Mono, SkeletonRows } from '../../components/ui'
import { Detail } from '../../components/workflow'
import { get, post } from '../../lib/api'
import { environments, formatDateTime } from '../../lib/format'
import { useCan, useMe } from '../../lib/session'
import { EnrichModal } from './EnrichModal'
import { Records } from './Records'
import { batchLabel, batchTone, formatLabel, skippedCount, templateFormat, useRefresh } from './shared'
import type { Batch, ImportRecord } from './shared'

type Action = 'approve' | 'reverse' | 'discard'

const done: Record<Action, string> = { approve: 'Import approved and committed.', reverse: 'Import reversed.', discard: 'Import discarded.' }

const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? '' : 's'}`

function confirmText(action: Action, batch: Batch) {
  const template = batch.format === templateFormat
  const count = plural(batch.summary.valid, template ? 'row' : 'subnet')
  const skipped = skippedCount(batch.summary)
  const left = skipped > 0 ? ` Skipped: ${plural(skipped, template ? 'row' : 'line')}, kept on record but not imported.` : ''
  if (action === 'approve') return {
    title: `Import ${count}?`, label: 'Import',
    body: template ? `Every row is checked again first, then all are added together, or none are.${left}` : `All valid lines become subnets together, or none do.${left}`,
  }
  if (action === 'discard') return { title: 'Discard this import?', label: 'Discard', body: 'Nothing reaches the inventory. Its rows stay on record, and the file can be uploaded again.' }
  return {
    title: 'Reverse this import?', label: 'Reverse',
    body: template ? 'Everything this import added is removed, children first. If a record is now in use, or an imported address has changed, nothing changes.'
      : 'The subnets this batch created are removed, if none has gained IP records or links since. Otherwise nothing changes.',
  }
}

function waitingNote(batch: Batch, own: boolean) {
  if (batch.summary.valid === 0) return batch.format === templateFormat ? 'No rows can be imported. Discard this, fix the file and upload it again.' : 'No lines can be imported yet.'
  return own ? 'Someone else must approve your upload.' : undefined
}

function Summary({ batch }: { batch: Batch }) {
  const s = batch.summary
  const template = batch.format === templateFormat
  const sheets = Object.entries(s.sheets ?? {})
  return (
    <>
      <div className={`mb-3 grid grid-cols-2 gap-x-6 ${template ? 'md:grid-cols-3' : 'md:grid-cols-4'}`}>
        <dl><Detail label={template ? 'Rows' : 'Lines'}>{s.lines}</Detail><Detail label="Ready">{s.valid}</Detail></dl>
        <dl><Detail label="Invalid">{s.invalid}</Detail><Detail label="Overlap">{s.overlap}</Detail></dl>
        <dl><Detail label={template ? 'Already exists' : 'Duplicate'}>{s.duplicate}</Detail><Detail label="Imported">{s.committed}</Detail></dl>
        {!template && <dl><Detail label="Needs enrichment">{s.needsEnrichment}</Detail><Detail label="Ignored (blank, comment)">{s.ignored}</Detail></dl>}
      </div>
      {sheets.length > 0 && <div className="mb-3 flex flex-wrap gap-1.5" aria-label="Rows to import by type">{sheets.map(([name, n]) => <Badge key={name} tone="accent">{name}: {n}</Badge>)}</div>}
    </>
  )
}

function Provenance({ batch }: { batch: Batch }) {
  const parts = [batch.routingDomainName, batch.environment && environments[batch.environment], `uploaded by ${batch.uploadedByEmail} ${formatDateTime(batch.createdAt)}`,
    batch.approvedByEmail && `approved by ${batch.approvedByEmail}`].filter(Boolean)
  return <p className="mb-3 text-[12.5px] text-ink-3">{parts.join(' · ')} · SHA-256 <Mono>{batch.fileHash.slice(0, 12)}…</Mono></p>
}

function BatchDetail({ batch, onClose }: { batch: Batch; onClose: () => void }) {
  const refresh = useRefresh()
  const me = useMe()
  const canRun = useCan('imports:run')
  const [confirm, setConfirm] = useState<Action | null>(null)
  const [enriching, setEnriching] = useState<ImportRecord | null>(null)
  const act = useMutation({ meta: { success: (a: Action) => done[a], inlineError: true }, mutationFn: (a: Action) => post<Batch>(`/imports/${batch.id}/${a}`), onSuccess: () => { refresh(); setConfirm(null); onClose() } })
  const ask = (a: Action) => { act.reset(); setConfirm(a) }
  const own = batch.uploadedBy === me.id
  const waiting = batch.status === 'validated'
  const template = batch.format === templateFormat
  const note = waiting ? waitingNote(batch, own) : undefined
  const text = confirm && confirmText(confirm, batch)
  return (
    <>
      <Modal open onClose={onClose} size="xl" title={batch.fileName} subtitle={<span className="flex items-center gap-2"><Badge tone={batchTone[batch.status]}>{batchLabel(batch.status)}</Badge>{formatLabel(batch.format)}</span>}
        footer={canRun && (waiting || batch.status === 'committed') && <>
          {note && <span className="mr-auto self-center text-[12.5px] text-ink-3">{note}</span>}
          {waiting && <Button onClick={() => ask('discard')}><Trash2 size={14} aria-hidden /> Discard</Button>}
          {waiting && !own && batch.summary.valid > 0 && <Button variant="primary" onClick={() => ask('approve')}>Import {plural(batch.summary.valid, template ? 'row' : 'subnet')}</Button>}
          {batch.status === 'committed' && <Button onClick={() => ask('reverse')}><Undo2 size={14} aria-hidden /> Reverse import</Button>}
        </>}>
        <Summary batch={batch} />
        <Provenance batch={batch} />
        {template && waiting && batch.summary.invalid + batch.summary.overlap > 0 && <p className="mb-3 text-[12.5px] text-ink-2">Template rows are corrected in the file. To change a row, discard this import, fix the file and upload it again.</p>}
        <Records path={`/imports/${batch.id}/preview`} batch={batch} onEnrich={canRun && waiting && !template ? setEnriching : undefined} />
      </Modal>
      {enriching && <EnrichModal record={enriching} onClose={() => setEnriching(null)} />}
      <ConfirmDialog open={confirm !== null} title={text?.title ?? ''} body={text?.body ?? ''} confirmLabel={text?.label ?? ''} tone={confirm === 'approve' ? 'primary' : 'danger'}
        busy={act.isPending} error={act.error} onConfirm={() => confirm && act.mutate(confirm)} onCancel={() => setConfirm(null)} />
    </>
  )
}

/** Loads the batch itself, so a link or a batch on another page still opens. */
export function BatchModal({ id, onClose }: { id: string; onClose: () => void }) {
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['imports', 'batch', id], queryFn: () => get<Batch>(`/imports/${id}`) })
  if (data) return <BatchDetail key={data.id} batch={data} onClose={onClose} />
  return (
    <Modal open onClose={onClose} size="xl" title="Import">
      {isPending && <SkeletonRows rows={6} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
    </Modal>
  )
}
