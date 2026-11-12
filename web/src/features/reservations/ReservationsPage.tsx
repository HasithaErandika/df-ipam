import { useMutation } from '@tanstack/react-query'
import { CalendarClock, CheckSquare, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router'
import { Badge, Button, Card, DataTable, FilterBar, FilterSelect, none, PageHeader, PathTabs, RowMenu, SearchInput, Stack, useConfirm, type Column } from '../../components/ui'
import { post } from '../../lib/api'
import { daysUntil, formatEndOfDay } from '../../lib/format'
import { useFilters, usePagedList } from '../../lib/listing'
import { inventoryKeys, useInvalidate, useSegments } from '../../lib/queries'
import { useCan, useMe } from '../../lib/session'
import { useRouteTab } from '../../lib/tabs'
import type { Reservation } from '../../lib/types'
import { ConvertReservation, EditReservation, NewReservation } from './ReservationForms'

const views: Record<string, string> = { reservations: 'active', expiring: 'expiring', expired: 'expired', closed: 'closed', all: 'all' }

function Expiry({ r }: { r: Reservation }) {
  if (!r.expiresAt) return <span className="text-ink-3">No expiry</span>
  const days = daysUntil(r.expiresAt)
  const tone = r.status !== 'active' ? 'neutral' : days <= 0 ? 'danger' : days <= 7 ? 'warn' : 'neutral'
  const text = r.status !== 'active' ? '' : days <= 0 ? 'Expired' : days === 1 ? 'Tomorrow' : `${days} days left`
  return <Stack main={formatEndOfDay(r.expiresAt)} sub={text ? <Badge tone={tone}>{text}</Badge> : undefined} />
}

export function ReservationsPage() {
  const tab = useRouteTab(Object.keys(views))
  const me = useMe()
  const canReserve = useCan('ip:reserve')
  const isAdmin = useCan('inventory:edit')
  const segments = useSegments()
  const f = useFilters(['segmentId', 'q', 'mine'] as const)
  const { query, rows, meta, paging } = usePagedList<Reservation>('/reservations', { ...f.values, view: views[tab] })
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<Reservation | null>(null)
  const [converting, setConverting] = useState<Reservation | null>(null)
  const confirm = useConfirm()
  const refresh = useInvalidate(...inventoryKeys)
  const release = useMutation({ mutationFn: (id: string) => post(`/reservations/${id}/release`), meta: { success: 'Reservation released.' }, onSuccess: () => void refresh() })
  const mayManage = (r: Reservation) => canReserve && r.status === 'active' && (isAdmin || r.ownerId === me.id)

  const columns: Column<Reservation>[] = [
    { key: 'range', header: 'Addresses', cell: (r) => <Stack mono main={r.startIp === r.endIp ? r.startIp : `${r.startIp} – ${r.endIp}`}
      sub={<span className="font-sans"><Link className="hover:text-accent hover:underline" to={`/segments/${r.segmentId}`}>{r.segmentName}</Link> · {r.size} address{r.size > 1 ? 'es' : ''}</span>} /> },
    { key: 'reason', header: 'Reason', cell: (r) => <span className="text-ink">{r.reason}</span> },
    { key: 'owner', header: 'Owner', hideBelow: 'md', cell: (r) => <Stack main={r.ownerId === me.id ? 'You' : r.ownerName || none} sub={r.createdByEmail && r.createdByEmail !== r.ownerName ? `by ${r.createdByEmail}` : undefined} /> },
    { key: 'expiry', header: 'Expires', cell: (r) => <Expiry r={r} /> },
    { key: 'status', header: 'Status', hideBelow: 'lg', cell: (r) => <Badge tone={r.status === 'active' ? 'warn' : r.status === 'converted' ? 'ok' : 'neutral'}>{r.status === 'active' ? 'Reserved' : r.status === 'converted' ? 'Assigned' : 'Released'}</Badge> },
    { key: 'act', header: '', className: 'w-10 text-right', cell: (r) => (mayManage(r) ? (
      <RowMenu actions={[
        { label: 'Extend or edit', icon: <CalendarClock size={14} aria-hidden />, onSelect: () => setEditing(r) },
        { label: 'Assign an address', icon: <CheckSquare size={14} aria-hidden />, onSelect: () => setConverting(r) },
        { label: 'Release', icon: <Trash2 size={14} aria-hidden />, danger: true, onSelect: async () => {
          if (await confirm({ title: `Release ${r.reason}?`, body: `${r.size} address${r.size > 1 ? 'es are' : ' is'} no longer held and can be assigned again. This is recorded in the audit log.`, confirmLabel: 'Release' })) release.mutate(r.id)
        } },
      ]} />
    ) : null) },
  ]

  return (
    <>
      <PageHeader title="Reservations" description="Hold an IP or a small range for a purpose. A reserved address seen in use raises an alert. Expired holds are never released automatically."
        actions={canReserve && <Button variant="primary" onClick={() => setCreating(true)}><Plus size={14} aria-hidden />Reserve addresses</Button>} />
      <PathTabs base="/reservations" tabs={[{ key: 'reservations', label: 'Active' }, { key: 'expiring', label: 'Expiring soon' }, { key: 'expired', label: 'Expired' }, { key: 'closed', label: 'Assigned or released' }, { key: 'all', label: 'All' }]} />
      <Card>
        <FilterBar active={f.active} onClear={f.clear}>
          <SearchInput value={f.values.q} onChange={(v) => f.set('q', v)} placeholder="Search reason, owner, IP" />
          <FilterSelect label="Segment" value={f.values.segmentId} onChange={(v) => f.set('segmentId', v)} options={(segments.data ?? []).map((s) => ({ value: s.id, text: s.name }))} />
          <FilterSelect label="Owner" value={f.values.mine} onChange={(v) => f.set('mine', v)} anyLabel="Anyone's" options={[{ value: 'true', text: 'Mine' }]} />
        </FilterBar>
        <DataTable columns={columns} rows={rows} query={query} rowKey={(r) => r.id} meta={meta} paging={paging} filtered={f}
          empty={{ title: tab === 'reservations' ? 'No active reservations' : 'Nothing here', body: 'Reserve an address or range to hold it for a purpose.' }} />
      </Card>
      <NewReservation open={creating} onClose={() => setCreating(false)} />
      <EditReservation hold={editing} onClose={() => setEditing(null)} />
      <ConvertReservation hold={converting} onClose={() => setConverting(null)} />
    </>
  )
}
