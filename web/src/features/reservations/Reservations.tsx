import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Search } from 'lucide-react'
import { useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { Badge, Button, Card, EmptyState, ErrorState, Field, FormError, Input, Modal, Mono, PageHeader, Pagination, Select, SkeletonRows, Td, Th, useConfirm } from '../../components/ui'
import { Detail, useChecks, useDebounced } from '../../components/workflow'
import { ApiError, get, list, post, queryString } from '../../lib/api'
import { daysUntil, formatDateTime, formatEndOfDay, isoDate } from '../../lib/format'
import { useClampPage, usePaging } from '../../lib/paging'
import { useCan } from '../../lib/session'
import type { Reservation } from '../../lib/types'
import { NewRequestModal, type Preset } from '../requests/NewRequestModal'
import { issues as check, required } from '../../lib/validation'

const statusLabels: Record<string, string> = { active: 'Active', expired: 'Expired', released: 'Released', converted: 'Converted to allocation', all: 'All' }

function ExpiryBadge({ r }: { r: Reservation }) {
  if (r.status !== 'active') return <Badge>{statusLabels[r.status]}</Badge>
  if (!r.expiresAt) return <Badge tone="warn">Reserved</Badge>
  const days = daysUntil(r.expiresAt)
  if (days <= 1) return <Badge tone="danger">Ends within a day</Badge>
  if (days <= 7) return <Badge tone="warn">Ends in {days} days</Badge>
  return <Badge tone="warn">Reserved</Badge>
}

function ReservationModal({ id, onClose, onConvert }: { id: string | null; onClose: () => void; onConvert: (r: Reservation) => void }) {
  const queryClient = useQueryClient()
  const canReserve = useCan('ip:reserve')
  const canRequest = useCan('ip:request')
  const [mode, setMode] = useState<'extend' | 'release' | null>(null)
  const [date, setDate] = useState('')
  const [reason, setReason] = useState('')
  const { data: r, isPending, error, refetch } = useQuery({ queryKey: ['reservation', id], enabled: Boolean(id), queryFn: () => get<Reservation>(`/reservations/${id}`) })
  const confirm = useConfirm()
  const act = useMutation({ meta: { success: mode === 'extend' ? 'Reservation extended.' : 'Reservation released.', inlineError: true },
    mutationFn: () => mode === 'extend'
      ? post<Reservation>(`/reservations/${id}/extend`, { expiryDate: date, reason })
      : post<Reservation>(`/reservations/${id}/release`, { reason }),
    onSuccess: () => {
      for (const key of ['reservation', 'reservations', 'summary', 'subnets']) queryClient.invalidateQueries({ queryKey: [key] })
      setMode(null)
      setReason('')
    },
  })
  const checks = useChecks(check({
    expiryDate: mode === 'extend' && !date ? 'Choose a new end date.' : undefined,
    reason: required(reason, 'Reason'),
  }))
  if (!id) return null
  const issues: Record<string, string> = {}
  if (act.error instanceof ApiError) for (const d of act.error.details) issues[d.field] = d.issue
  Object.assign(issues, checks.shown)
  const active = r?.status === 'active'
  return (
    <Modal open onClose={onClose} title="Reservation" subtitle={r && <span className="flex items-center gap-2"><ExpiryBadge r={r} /> <Mono>{r.address}</Mono></span>} size="lg"
      footer={r && active && !mode && (
        <>
          {canRequest && <Button onClick={() => onConvert(r)}>Convert to allocation</Button>}
          {canReserve && <Button onClick={() => { setMode('release'); act.reset() }}>Release</Button>}
          {canReserve && <Button variant="primary" onClick={() => { setMode('extend'); setDate(isoDate(30)); act.reset() }}>Extend</Button>}
        </>
      )}>
      {isPending && <SkeletonRows rows={5} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {r && (
        <>
          <dl>
            <Detail label="Subnet">{r.subnetName} (<Mono>{r.subnetCidr}</Mono>), {r.routingDomainName}</Detail>
            <Detail label="Held by">{r.reservedByEmail}</Detail>
            <Detail label="Reason">{r.reason || '—'}</Detail>
            <Detail label="Reserved">{formatDateTime(r.reservedAt)}</Detail>
            <Detail label="Ends">{r.expiresAt ? `${formatEndOfDay(r.expiresAt)} (UTC)` : 'No end date'}</Detail>
            {r.releasedAt && <Detail label={r.status === 'expired' ? 'Expired' : 'Ended'}>{formatDateTime(r.releasedAt)}{r.releasedByEmail && ` by ${r.releasedByEmail}`}</Detail>}
            {r.requestId && <Detail label="Request"><Link className="text-accent hover:underline" to={`/requests?id=${r.requestId}`}>Open the request</Link></Detail>}
          </dl>
          {mode && (
            <div className="mt-4 space-y-3 rounded-[5px] border border-line bg-surface-2 p-3">
              {mode === 'extend' && (
                <Field label="New end date" htmlFor="rv-date" error={issues.expiryDate} hint="At most a year from today.">
                  <Input id="rv-date" type="date" min={isoDate(1)} max={isoDate(365)} value={date} onChange={(e) => setDate(e.target.value)} />
                </Field>
              )}
              <Field label="Reason" htmlFor="rv-reason" error={issues.reason}>
                <Input id="rv-reason" value={reason} onChange={(e) => setReason(e.target.value)}
                  placeholder={mode === 'extend' ? 'Why it needs longer' : 'Why it is no longer needed'} />
              </Field>
              {mode === 'release' && <p className="text-[12.5px] text-ink-2">The address becomes an available candidate again. The holder is told.</p>}
              {Object.keys(issues).length === 0 && <FormError error={act.error} />}
              <div className="flex justify-end gap-2">
                <Button onClick={() => setMode(null)}>Back</Button>
                <Button variant={mode === 'release' ? 'danger' : 'primary'} busy={act.isPending} onClick={() => checks.submit(async () => { if (mode === 'extend' || await confirm({ title: 'Release this reservation?', body: 'The address returns to the available candidates, and the holder is told.', confirmLabel: 'Release' })) act.mutate() })}>
                  {mode === 'extend' ? 'Save new end date' : 'Release reservation'}
                </Button>
              </div>
            </div>
          )}
        </>
      )}
    </Modal>
  )
}

export function Reservations() {
  const canRequest = useCan('ip:request')
  const [params, setParams] = useSearchParams()
  const paging = usePaging()
  const [status, setStatus] = useState('active')
  const [expiring, setExpiring] = useState('')
  const [search, setSearch] = useState('')
  const [creating, setCreating] = useState<Preset | null>(null)
  const q = useDebounced(search.trim())
  const openId = params.get('id')
  const setOpen = (id: string | null) => setParams((prev) => {
    const next = new URLSearchParams(prev)
    if (id) next.set('id', id)
    else next.delete('id')
    return next
  })
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, status, expiringWithinDays: status === 'active' ? expiring : '', q })
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['reservations', qs], placeholderData: keepPreviousData, queryFn: () => list<Reservation>(`/reservations${qs}`),
  })
  useClampPage(paging, data?.meta)
  const filtered = Boolean(q || expiring || status !== 'active')
  return (
    <>
      <PageHeader title="Reservations" description="Addresses held for planned work. Holders are reminded a week before a reservation ends, and the address returns to the candidate pool when it does."
        actions={canRequest && <Button variant="primary" onClick={() => setCreating({ requestType: 'reservation' })}><Plus size={15} aria-hidden /> Request a reservation</Button>} />
      <Card>
        <div className="flex flex-wrap items-center gap-2 border-b border-line p-2.5">
          <div className="relative min-w-52 flex-1">
            <Search size={14} className="absolute top-1/2 left-2.5 -translate-y-1/2 text-ink-3" aria-hidden />
            <Input aria-label="Search reservations" placeholder="Address prefix or subnet name" className="pl-8" value={search} onChange={(e) => { setSearch(e.target.value); paging.setPage(1) }} />
          </div>
          <Select aria-label="Status" className="w-52!" value={status} onChange={(e) => { setStatus(e.target.value); paging.setPage(1) }}>
            {Object.entries(statusLabels).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </Select>
          {status === 'active' && (
            <Select aria-label="Ending" className="w-44!" value={expiring} onChange={(e) => { setExpiring(e.target.value); paging.setPage(1) }}>
              <option value="">Any end date</option><option value="7">Ending within 7 days</option><option value="30">Ending within 30 days</option>
            </Select>
          )}
        </div>
        {isPending && <SkeletonRows rows={8} />}
        {error && <ErrorState error={error} onRetry={refetch} />}
        {data?.data.length === 0 && (filtered
          ? <EmptyState title="No reservations match these filters" />
          : <EmptyState title="No active reservations" body="Approved reservation requests appear here once fulfilled." />)}
        {data && data.data.length > 0 && (
          <>
            <div className="overflow-x-auto">
              <table className="w-full border-collapse">
                <thead><tr><Th>Address</Th><Th>Subnet</Th><Th>Held by</Th><Th>Reason</Th><Th>Ends</Th><Th>Status</Th></tr></thead>
                <tbody>
                  {data.data.map((r) => (
                    <tr key={r.id} className="cursor-pointer hover:bg-surface-2" onClick={() => setOpen(r.id)}>
                      <Td><button type="button" className="cursor-pointer hover:text-accent" onClick={() => setOpen(r.id)}><Mono>{r.address}</Mono></button></Td>
                      <Td className="whitespace-nowrap text-ink-2">{r.subnetName} <Mono>{r.subnetCidr}</Mono></Td>
                      <Td className="text-ink-2">{r.reservedByEmail}</Td>
                      <Td className="max-w-72 truncate text-ink-2">{r.reason}</Td>
                      <Td className="whitespace-nowrap text-ink-2">{r.expiresAt ? formatEndOfDay(r.expiresAt) : '—'}</Td>
                      <Td><ExpiryBadge r={r} /></Td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
          </>
        )}
      </Card>
      <ReservationModal id={openId} onClose={() => setOpen(null)}
        onConvert={(r) => { setOpen(null); setCreating({ requestType: 'allocation', reservationId: r.id, reservationAddress: r.address }) }} />
      {creating && <NewRequestModal open preset={creating} onClose={() => setCreating(null)} />}
    </>
  )
}
