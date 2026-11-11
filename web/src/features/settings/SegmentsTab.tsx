import { useMutation } from '@tanstack/react-query'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router'
import { Badge, Button, Card, DataTable, EnvironmentBadge, RowMenu, Stack, useConfirm, type Column } from '../../components/ui'
import { del } from '../../lib/api'
import { formatCount } from '../../lib/format'
import { label, segmentKinds } from '../../lib/labels'
import { inventoryKeys, useInvalidate, useSegments } from '../../lib/queries'
import type { Segment } from '../../lib/types'
import { SegmentForm } from './SegmentForm'

export function SegmentsTab() {
  const q = useSegments()
  const [form, setForm] = useState<{ segment?: Segment } | null>(null)
  const confirm = useConfirm()
  const refresh = useInvalidate(...inventoryKeys)
  const remove = useMutation({ mutationFn: (s: Segment) => del(`/segments/${s.id}`), meta: { success: (s: Segment) => `${s.name} deleted.` }, onSuccess: () => void refresh() })
  const columns: Column<Segment>[] = [
    { key: 'name', header: 'Segment', cell: (s) => <Stack main={<Link className="font-medium hover:text-accent hover:underline" to={`/segments/${s.id}`}>{s.name}</Link>} sub={s.description || undefined} /> },
    { key: 'kind', header: 'Type', cell: (s) => <Badge tone={s.kind === 'dhcp' ? 'accent' : s.kind === 'cloud' ? 'neutral' : 'warn'}>{label(segmentKinds, s.kind)}</Badge> },
    { key: 'network', header: 'Network', cell: (s) => <Stack mono main={s.network} sub={[s.floor !== null ? `Floor ${s.floor}` : null, s.vlan !== null ? `VLAN ${s.vlan}` : null].filter(Boolean).join(' · ') || undefined} /> },
    { key: 'pool', header: 'DHCP pool / environment', hideBelow: 'md', cell: (s) => (s.kind === 'dhcp'
      ? (s.dhcpStart ? <span className="font-mono text-[12px]">{s.dhcpStart} – {s.dhcpEnd}</span> : <span className="text-warn">No pool set</span>)
      : <EnvironmentBadge name={s.environmentName} />) },
    { key: 'use', header: 'Registered / seen', hideBelow: 'lg', className: 'tabular-nums', cell: (s) => `${formatCount(s.registered)} / ${formatCount(s.seen)}` },
    { key: 'size', header: 'Usable', hideBelow: 'lg', className: 'text-right tabular-nums', cell: (s) => formatCount(s.usableSize) },
    { key: 'act', header: '', className: 'w-10 text-right', cell: (s) => (
      <RowMenu actions={[
        { label: 'Edit', icon: <Pencil size={14} aria-hidden />, onSelect: () => setForm({ segment: s }) },
        { label: 'Delete', icon: <Trash2 size={14} aria-hidden />, danger: true, onSelect: async () => {
          if (await confirm({ title: `Delete ${s.name}?`, body: 'Only an empty segment can be deleted: one with no register entries, reservations or requests. Its scan history is kept without a segment.', confirmLabel: 'Delete segment' })) remove.mutate(s)
        } },
      ]} />
    ) },
  ]
  return (
    <Card title="Segments" subtitle="Floors, server networks and cloud networks. Networks never overlap."
      action={<Button variant="primary" onClick={() => setForm({})}><Plus size={14} aria-hidden />Add segment</Button>}>
      <DataTable columns={columns} rows={q.data ?? []} query={q} rowKey={(s) => s.id}
        empty={{ title: 'No segments yet', body: 'Add one per floor (with its DHCP pool), and one per server or cloud network.' }} />
      {q.data && q.data.length > 0 && <p className="border-t border-line px-4 py-2 text-[12px] text-ink-3">Segments over 4,096 addresses are not swept; split them for discovery.</p>}
      <SegmentForm open={form !== null} segment={form?.segment} onClose={() => setForm(null)} />
    </Card>
  )
}
