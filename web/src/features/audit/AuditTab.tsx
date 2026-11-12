import { useState } from 'react'
import { Badge, Button, Card, DataTable, Details, FilterBar, FilterSelect, Input, Modal, none, SearchInput, Stack, type Column } from '../../components/ui'
import { formatDateTime } from '../../lib/format'
import { useFilters, usePagedList } from '../../lib/listing'
import type { AuditEntry } from '../../lib/types'
import { ExportMenu } from '../shared/FileTransfer'

const objects = ['segment', 'ip_address', 'device', 'reservation', 'ip_request', 'alert', 'environment', 'department', 'user', 'role', 'settings', 'integration', 'discovery']
const actions = ['create', 'update', 'delete', 'release', 'reserve', 'convert', 'approved', 'rejected', 'cancel', 'acknowledge', 'import', 'scan', 'sync']
const nice = (s: string) => (s.charAt(0).toUpperCase() + s.slice(1)).replaceAll('_', ' ')
const tone = (a: string) => (['delete', 'release', 'rejected', 'cancel'].includes(a) ? 'danger' : ['create', 'approved', 'import'].includes(a) ? 'ok' : 'neutral')

function Values({ value }: { value: unknown }) {
  if (value === null || value === undefined) return <p className="text-[12.5px] text-ink-3">Nothing</p>
  return <pre className="max-h-64 overflow-auto rounded-md bg-surface-2 p-3 font-mono text-[11.5px] leading-relaxed text-ink-2">{JSON.stringify(value, null, 2)}</pre>
}

export function AuditTab() {
  const f = useFilters(['user', 'action', 'objectType', 'from', 'to', 'q'] as const)
  const { query, rows, meta, paging } = usePagedList<AuditEntry>('/audit-logs', f.values)
  const [open, setOpen] = useState<AuditEntry | null>(null)
  const columns: Column<AuditEntry>[] = [
    { key: 'when', header: 'When', cell: (e) => <span className="whitespace-nowrap">{formatDateTime(e.occurredAt)}</span> },
    { key: 'who', header: 'Who', cell: (e) => e.actorEmail ?? <Badge>System</Badge> },
    { key: 'what', header: 'What', cell: (e) => <div className="flex items-center gap-2"><Badge tone={tone(e.action)}>{nice(e.action)}</Badge><span>{nice(e.objectType)}</span></div> },
    { key: 'id', header: 'Record', hideBelow: 'lg', cell: (e) => (e.objectId ? <span className="font-mono text-[11.5px] text-ink-3">{e.objectId}</span> : none) },
    { key: 'ip', header: 'From', hideBelow: 'md', cell: (e) => (e.sourceIp ? <span className="font-mono text-[12px]">{e.sourceIp}</span> : none) },
    { key: 'open', header: '', className: 'text-right', cell: (e) => <Button variant="ghost" onClick={() => setOpen(e)}>Details</Button> },
  ]
  return (
    <Card title="Audit log" subtitle="Every change, written in the same transaction as the change. It can't be edited or deleted."
      action={<ExportMenu path="/audit-logs/export" params={Object.fromEntries(Object.entries(f.values).filter(([, v]) => v))} name="audit-log" />}>
      <FilterBar active={f.active} onClear={f.clear}>
        <SearchInput value={f.values.q} onChange={(v) => f.set('q', v)} placeholder="Search IP, MAC, name or record ID" />
        <SearchInput value={f.values.user} onChange={(v) => f.set('user', v)} placeholder="User email" className="w-48" />
        <FilterSelect label="Action" value={f.values.action} onChange={(v) => f.set('action', v)} anyLabel="Any action" options={actions.map((a) => ({ value: a, text: nice(a) }))} />
        <FilterSelect label="Record type" value={f.values.objectType} onChange={(v) => f.set('objectType', v)} anyLabel="Any record" options={objects.map((o) => ({ value: o, text: nice(o) }))} />
        <label className="flex items-center gap-1.5 text-[12.5px] text-ink-3">From<Input type="date" className="h-8 w-36" value={f.values.from} onChange={(e) => f.set('from', e.target.value)} /></label>
        <label className="flex items-center gap-1.5 text-[12.5px] text-ink-3">To<Input type="date" className="h-8 w-36" value={f.values.to} min={f.values.from || undefined} onChange={(e) => f.set('to', e.target.value)} /></label>
      </FilterBar>
      <DataTable columns={columns} rows={rows} query={query} rowKey={(e) => String(e.id)} meta={meta} paging={paging} filtered={f} empty={{ title: 'No changes recorded yet' }} />
      <Modal open={open !== null} onClose={() => setOpen(null)} size="xl" title={open ? `${nice(open.action)} ${nice(open.objectType).toLowerCase()}` : ''} subtitle={open ? formatDateTime(open.occurredAt) : undefined}
        footer={<Button onClick={() => setOpen(null)}>Close</Button>}>
        {open && (
          <div className="space-y-4">
            <Details items={[['Who', open.actorEmail ?? 'System'], ['From', open.sourceIp ?? '—'], ['Record ID', <span key="id" className="font-mono text-[12px]">{open.objectId ?? '—'}</span>], ['Reference', <span key="c" className="font-mono text-[12px]">{open.correlationId ?? '—'}</span>]]} />
            <div className="grid gap-3 lg:grid-cols-2">
              <Stack main={<span className="text-[12px] font-semibold text-ink-3">Before</span>} />
              <Stack main={<span className="text-[12px] font-semibold text-ink-3">After</span>} />
              <Values value={open.oldValues} />
              <Values value={open.newValues} />
            </div>
          </div>
        )}
      </Modal>
    </Card>
  )
}
