import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Check, Plus, Search, X } from 'lucide-react'
import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router'
import {
  Badge, Button, Card, ConfirmDialog, EmptyState, ErrorState, Field, FormError, Input, Modal, Mono, PageHeader, Pagination,
  Select, SkeletonRows, Td, Th,
} from '../components/ui'
import { Detail, StatusBadge, SubnetPicker, Tabs, useDebounced, type TabDef } from '../components/workflow'
import { ApiError, del, get, list, listAll, post, put, queryString } from '../lib/api'
import { environments, formatDate, formatDateTime, formatEndOfDay, isoDate, requestStatuses, requestTypes } from '../lib/format'
import { useClampPage, usePaging } from '../lib/paging'
import { useCan, useMe } from '../lib/session'
import type { Allocation, IPRequest, PendingApproval, RequestType, Role, Subnet, Workflow } from '../lib/types'

/** Invalidates everything a workflow change can affect. */
function useRefresh() {
  const queryClient = useQueryClient()
  return () => {
    for (const key of ['requests', 'request', 'approvals', 'allocations', 'reservations', 'summary', 'notifications', 'subnets']) {
      queryClient.invalidateQueries({ queryKey: [key] })
    }
  }
}

function fieldErrors(error: unknown): Record<string, string> {
  const out: Record<string, string> = {}
  if (error instanceof ApiError) for (const d of error.details) out[d.field] = d.issue
  return out
}

export interface Preset { requestType: RequestType; allocation?: Allocation; reservationId?: string; reservationAddress?: string }

export function NewRequestModal({ open, preset, onClose }: { open: boolean; preset?: Preset; onClose: () => void }) {
  const refresh = useRefresh()
  const navigate = useNavigate()
  const [requestType, setRequestType] = useState<RequestType>(preset?.requestType ?? 'allocation')
  const [subnet, setSubnet] = useState<Subnet | null>(null)
  const [mode, setMode] = useState<'automatic' | 'specific'>('automatic')
  const [address, setAddress] = useState('')
  const [justification, setJustification] = useState('')
  const [department, setDepartment] = useState('')
  const [environment, setEnvironment] = useState('')
  const [requiredDate, setRequiredDate] = useState('')
  const [expiryDate, setExpiryDate] = useState(isoDate(30))
  const [comments, setComments] = useState('')
  const release = requestType === 'release'
  const conversion = Boolean(preset?.reservationId)

  const save = useMutation({
    mutationFn: (submit: boolean) => post<IPRequest>('/requests', {
      requestType, submit, justification, department, comments,
      subnetId: subnet?.id ?? '', selectionMode: mode, requestedAddress: mode === 'specific' ? address : null,
      allocationId: preset?.allocation?.id ?? null, reservationId: preset?.reservationId ?? null,
      environment: environment || null, requiredDate: requiredDate || null,
      expiryDate: requestType === 'reservation' ? expiryDate : null,
    }),
    onSuccess: (r) => {
      refresh()
      onClose()
      navigate(`/requests?tab=mine&id=${r.id}`)
    },
  })
  const errors = fieldErrors(save.error)
  const fieldLevel = Object.keys(errors).length > 0
  const title = conversion ? 'Convert reservation to allocation' : release ? 'Request a release' : 'New request'

  return (
    <Modal open={open} onClose={onClose} title={title} size="lg"
      subtitle="Requests are approved before anything changes. You can't approve your own."
      footer={<>
        <Button onClick={onClose}>Cancel</Button>
        <Button busy={save.isPending && save.variables === false} disabled={save.isPending} onClick={() => save.mutate(false)}>Save draft</Button>
        <Button variant="primary" busy={save.isPending && save.variables === true} disabled={save.isPending} onClick={() => save.mutate(true)}>Submit for approval</Button>
      </>}>
      <div className="space-y-4">
        {!preset && (
          <Field label="Request type" htmlFor="rq-type">
            <Select id="rq-type" value={requestType} onChange={(e) => setRequestType(e.target.value as RequestType)}>
              <option value="allocation">Allocation: give an address to a use</option>
              <option value="reservation">Reservation: hold an address for planned work</option>
            </Select>
          </Field>
        )}
        {release && preset?.allocation && (
          <dl className="rounded-[5px] border border-line bg-surface-2 px-3 py-1">
            <Detail label="Address"><Mono>{preset.allocation.address}</Mono></Detail>
            <Detail label="Subnet">{preset.allocation.subnetName} (<Mono>{preset.allocation.subnetCidr}</Mono>)</Detail>
            <Detail label="Allocated">{formatDateTime(preset.allocation.allocatedAt)} by {preset.allocation.allocatedByEmail}</Detail>
          </dl>
        )}
        {conversion && (
          <p className="rounded-[5px] bg-accent-soft px-3 py-2 text-[13px] text-accent">
            Once approved and fulfilled, reserved address <Mono>{preset?.reservationAddress}</Mono> becomes an allocation.
          </p>
        )}
        {!release && !conversion && (
          <>
            <Field label="Subnet" htmlFor="rq-subnet" error={errors.subnetId}>
              <SubnetPicker id="rq-subnet" value={subnet} invalid={Boolean(errors.subnetId)} onChange={(s) => { setSubnet(s); if (s && !environment) setEnvironment(s.environment) }} />
            </Field>
            <div className="grid grid-cols-[15rem_1fr] gap-3">
              <Field label="Address" htmlFor="rq-mode" error={errors.selectionMode}>
                <Select id="rq-mode" value={mode} onChange={(e) => setMode(e.target.value as 'automatic' | 'specific')}>
                  <option value="automatic">Next available candidate</option>
                  <option value="specific">A specific address</option>
                </Select>
              </Field>
              {mode === 'specific' && (
                <Field label="Specific address" htmlFor="rq-address" error={errors.requestedAddress}>
                  <Input id="rq-address" className="font-mono" spellCheck={false} placeholder={subnet?.calculated.firstUsable ?? '10.20.30.40'}
                    value={address} onChange={(e) => setAddress(e.target.value)} />
                </Field>
              )}
            </div>
          </>
        )}
        <Field label="Justification" htmlFor="rq-justification" error={errors.justification} hint="What the address is for. Approvers see this.">
          <textarea id="rq-justification" rows={3} value={justification} onChange={(e) => setJustification(e.target.value)}
            className="w-full rounded-[5px] border border-line-strong bg-surface-2 px-2.5 py-1.5 text-[13px] text-ink focus:border-accent focus:bg-surface focus:outline-none" />
        </Field>
        {!release && (
          <div className="grid grid-cols-2 gap-3">
            <Field label="Department (optional)" htmlFor="rq-dept"><Input id="rq-dept" value={department} onChange={(e) => setDepartment(e.target.value)} /></Field>
            <Field label="Environment" htmlFor="rq-env" error={errors.environment}>
              <Select id="rq-env" value={environment} onChange={(e) => setEnvironment(e.target.value)}>
                <option value="">Same as the subnet</option>
                {Object.entries(environments).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
              </Select>
            </Field>
            <Field label="Needed by (optional)" htmlFor="rq-required" error={errors.requiredDate}>
              <Input id="rq-required" type="date" value={requiredDate} onChange={(e) => setRequiredDate(e.target.value)} />
            </Field>
            {requestType === 'reservation' && (
              <Field label="Reservation ends" htmlFor="rq-expiry" error={errors.expiryDate} hint="At most a year ahead. The holder is reminded a week before.">
                <Input id="rq-expiry" type="date" min={isoDate(1)} max={isoDate(365)} value={expiryDate} onChange={(e) => setExpiryDate(e.target.value)} />
              </Field>
            )}
          </div>
        )}
        <Field label="Comments (optional)" htmlFor="rq-comments"><Input id="rq-comments" value={comments} onChange={(e) => setComments(e.target.value)} /></Field>
        {!fieldLevel && <FormError error={save.error} />}
      </div>
    </Modal>
  )
}

function decisionTone(d: string) {
  return d === 'approved' ? 'ok' as const : d === 'rejected' ? 'danger' as const : 'neutral' as const
}

function RequestModal({ id, onClose }: { id: string | null; onClose: () => void }) {
  const me = useMe()
  const refresh = useRefresh()
  const canAllocate = useCan('ip:allocate')
  const canReserve = useCan('ip:reserve')
  const [comment, setComment] = useState('')
  const [confirm, setConfirm] = useState<'cancel' | 'delete' | null>(null)
  const { data: r, isPending, error, refetch } = useQuery({
    queryKey: ['request', id], enabled: Boolean(id), queryFn: () => get<IPRequest>(`/requests/${id}`),
  })
  const act = useMutation({
    mutationFn: async (action: string) => {
      if (!r) return
      switch (action) {
        case 'submit': return post(`/requests/${r.id}/submit`)
        case 'cancel': return post(`/requests/${r.id}/cancel`)
        case 'delete': return del(`/requests/${r.id}`)
        case 'approve': return post(`/approvals/${r.myApprovalId}/approve`, { comment })
        case 'reject': return post(`/approvals/${r.myApprovalId}/reject`, { comment })
        case 'fulfil':
          if (r.requestType === 'allocation') return post('/allocations', { requestId: r.id })
          if (r.requestType === 'reservation') return post('/reservations', { requestId: r.id })
          return post(`/allocations/${r.allocationId}/release`, { requestId: r.id })
      }
    },
    onSuccess: (_, action) => {
      refresh()
      setComment('')
      setConfirm(null)
      if (action === 'delete') onClose()
    },
  })
  if (!id) return null
  const mine = r?.requestedBy === me.id
  const canFulfil = r?.status === 'approved' && (r.requestType === 'reservation' ? canReserve : canAllocate)
  const fulfilLabel = r?.requestType === 'reservation' ? 'Reserve address' : r?.requestType === 'release' ? 'Release address' : 'Allocate address'
  const errors = fieldErrors(act.error)

  return (
    <>
      <Modal open onClose={onClose} size="lg"
        title={r ? `${requestTypes[r.requestType]} request` : 'Request'}
        subtitle={r && <span className="flex items-center gap-2"><StatusBadge status={r.status} /> <Mono>{r.result?.address ?? r.requestedAddress ?? r.subnetCidr}</Mono></span>}
        footer={r && (
          <>
            {mine && r.status === 'draft' && <Button onClick={() => setConfirm('delete')}>Delete draft</Button>}
            {mine && ['draft', 'pending_approval', 'approved'].includes(r.status) && <Button onClick={() => setConfirm('cancel')}>Cancel request</Button>}
            {mine && r.status === 'draft' && <Button variant="primary" busy={act.isPending} onClick={() => act.mutate('submit')}>Submit for approval</Button>}
            {canFulfil && <Button variant="primary" busy={act.isPending} onClick={() => act.mutate('fulfil')}>{fulfilLabel}</Button>}
            {!(mine && ['draft', 'pending_approval', 'approved'].includes(r.status)) && !canFulfil && <Button onClick={onClose}>Close</Button>}
          </>
        )}>
        {isPending && <SkeletonRows rows={6} />}
        {error && <ErrorState error={error} onRetry={refetch} />}
        {r && (
          <>
            {r.result && (
              <div className="mb-4 rounded-[5px] border border-ok/30 bg-ok-soft px-3 py-2 text-[13px] text-ok">
                {r.requestType === 'release' ? 'Allocation' : r.result.kind === 'reservation' ? 'Reserved' : 'Allocated'} <Mono>{r.result.address}</Mono>
                {r.result.expiresAt && <> until {formatEndOfDay(r.result.expiresAt).replace('End of ', 'the end of ')}</>} · status {r.result.status}
              </div>
            )}
            <dl>
              <Detail label="Requested by">{r.requestedByEmail} on {formatDateTime(r.createdAt)}</Detail>
              <Detail label="Subnet">{r.subnetName} (<Mono>{r.subnetCidr}</Mono>), {r.routingDomainName}</Detail>
              {r.requestType !== 'release' && <Detail label="Address">{r.selectionMode === 'automatic' ? 'Next available candidate' : <Mono>{r.requestedAddress}</Mono>}</Detail>}
              <Detail label="Justification">{r.justification}</Detail>
              {r.department && <Detail label="Department">{r.department}</Detail>}
              {r.environment && <Detail label="Environment">{environments[r.environment] ?? r.environment}</Detail>}
              {r.requiredDate && <Detail label="Needed by">{formatDate(r.requiredDate)}</Detail>}
              {r.expiryDate && <Detail label="Reservation ends">{formatDate(r.expiryDate)}</Detail>}
              {r.comments && <Detail label="Comments">{r.comments}</Detail>}
              {r.workflowName && <Detail label="Workflow">{r.workflowName}</Detail>}
              {r.fulfilledAt && <Detail label="Fulfilled">{formatDateTime(r.fulfilledAt)} by {r.fulfilledByEmail}</Detail>}
            </dl>

            {r.approvals.length > 0 && (
              <>
                <h3 className="mt-5 mb-1.5 text-[13px] font-semibold text-ink">Approval steps</h3>
                <ol className="space-y-1.5">
                  {r.approvals.map((a) => (
                    <li key={a.id} className="flex items-start justify-between gap-3 rounded-[5px] border border-line px-3 py-2 text-[13px]">
                      <div className="min-w-0">
                        <div className="font-medium text-ink">Step {a.stepOrder}: {a.approverRoleName ?? 'Any approver'}</div>
                        {a.approverEmail && <div className="text-ink-2">{a.approverEmail}{a.decidedAt && <> · {formatDateTime(a.decidedAt)}</>}</div>}
                        {a.comment && <div className="mt-0.5 text-ink-2">“{a.comment}”</div>}
                      </div>
                      <Badge tone={decisionTone(a.decision)}>{a.decision}</Badge>
                    </li>
                  ))}
                </ol>
              </>
            )}

            {r.myApprovalId && (
              <div className="mt-4 rounded-[5px] border border-accent/40 bg-accent-soft/40 p-3">
                <Field label="Your decision" htmlFor="decision-comment" error={errors.comment} hint="A comment is required to reject.">
                  <Input id="decision-comment" placeholder="Comment" value={comment} onChange={(e) => setComment(e.target.value)} />
                </Field>
                <div className="mt-2 flex justify-end gap-2">
                  <Button busy={act.isPending && act.variables === 'reject'} disabled={act.isPending} onClick={() => act.mutate('reject')}><X size={14} aria-hidden /> Reject</Button>
                  <Button variant="primary" busy={act.isPending && act.variables === 'approve'} disabled={act.isPending} onClick={() => act.mutate('approve')}><Check size={14} aria-hidden /> Approve</Button>
                </div>
              </div>
            )}
            {mine && r.status === 'pending_approval' && <p className="mt-3 text-[12.5px] text-ink-3">Waiting for another approver. You can't approve your own request.</p>}
            {Object.keys(errors).length === 0 && !confirm && <div className="mt-3"><FormError error={act.error} /></div>}
          </>
        )}
      </Modal>
      <ConfirmDialog open={confirm !== null} title={confirm === 'delete' ? 'Delete this draft?' : 'Cancel this request?'}
        body={confirm === 'delete' ? 'The draft is removed. This is recorded in the audit log.' : 'The request stops here. Any pending approvals are no longer needed.'}
        confirmLabel={confirm === 'delete' ? 'Delete' : 'Cancel request'} busy={act.isPending} error={act.error}
        onConfirm={() => confirm && act.mutate(confirm)} onCancel={() => setConfirm(null)} />
    </>
  )
}

function RequestsTable({ mine, onOpen }: { mine: boolean; onOpen: (id: string) => void }) {
  const paging = usePaging(mine ? 'mine' : 'all')
  const [status, setStatus] = useState(mine ? '' : 'open')
  const [type, setType] = useState('')
  const [search, setSearch] = useState('')
  const q = useDebounced(search.trim())
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, mine, status, type, q })
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['requests', qs], placeholderData: keepPreviousData, queryFn: () => list<IPRequest>(`/requests${qs}`),
  })
  useClampPage(paging, data?.meta)
  const filtered = Boolean(status || type || q)
  return (
    <Card>
      <div className="flex flex-wrap items-center gap-2 border-b border-line p-2.5">
        <div className="relative min-w-52 flex-1">
          <Search size={14} className="absolute top-1/2 left-2.5 -translate-y-1/2 text-ink-3" aria-hidden />
          <Input aria-label="Search requests" placeholder="Search justification, address, subnet or requester" className="pl-8" value={search}
            onChange={(e) => { setSearch(e.target.value); paging.setPage(1) }} />
        </div>
        <Select aria-label="Status" className="w-44!" value={status} onChange={(e) => { setStatus(e.target.value); paging.setPage(1) }}>
          <option value="">All statuses</option>
          <option value="open">In progress</option>
          {Object.entries(requestStatuses).filter(([k]) => k !== 'submitted').map(([k, v]) => <option key={k} value={k}>{v}</option>)}
        </Select>
        <Select aria-label="Type" className="w-36!" value={type} onChange={(e) => { setType(e.target.value); paging.setPage(1) }}>
          <option value="">All types</option>
          {Object.entries(requestTypes).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
        </Select>
      </div>
      {isPending && <SkeletonRows rows={8} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && (filtered
        ? <EmptyState title="No requests match these filters" />
        : <EmptyState title={mine ? "You haven't made any requests" : 'No requests yet'} body="Use New request to ask for an allocation or reservation." />)}
      {data && data.data.length > 0 && (
        <>
          <div className="overflow-x-auto">
            <table className="w-full border-collapse">
              <thead><tr><Th>Type</Th><Th>Address</Th><Th>Subnet</Th>{!mine && <Th>Requested by</Th>}<Th>Justification</Th><Th>Created</Th><Th>Status</Th></tr></thead>
              <tbody>
                {data.data.map((r) => (
                  <tr key={r.id} className="cursor-pointer hover:bg-surface-2" onClick={() => onOpen(r.id)}>
                    <Td><button type="button" className="cursor-pointer font-medium text-ink hover:text-accent" onClick={() => onOpen(r.id)}>{requestTypes[r.requestType]}</button></Td>
                    <Td>{r.requestedAddress ? <Mono>{r.requestedAddress}</Mono> : <span className="text-ink-3">Automatic</span>}</Td>
                    <Td className="whitespace-nowrap text-ink-2">{r.subnetName} <Mono>{r.subnetCidr}</Mono></Td>
                    {!mine && <Td className="text-ink-2">{r.requestedByEmail}</Td>}
                    <Td className="max-w-80 truncate text-ink-2">{r.justification}</Td>
                    <Td className="whitespace-nowrap text-ink-2">{formatDateTime(r.createdAt)}</Td>
                    <Td><StatusBadge status={r.status} /></Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
        </>
      )}
    </Card>
  )
}

function ApprovalsTable({ onOpen }: { onOpen: (id: string) => void }) {
  const paging = usePaging('approvals')
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize })
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['approvals', qs], placeholderData: keepPreviousData, queryFn: () => list<PendingApproval>(`/approvals${qs}`),
  })
  useClampPage(paging, data?.meta)
  return (
    <Card>
      {isPending && <SkeletonRows rows={6} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && <EmptyState title="Nothing waiting for you" body="Steps you can decide appear here. Your own requests never do." />}
      {data && data.data.length > 0 && (
        <>
          <div className="overflow-x-auto">
            <table className="w-full border-collapse">
              <thead><tr><Th>Request</Th><Th>Address</Th><Th>Subnet</Th><Th>Requested by</Th><Th>Step</Th><Th>Submitted</Th><Th /></tr></thead>
              <tbody>
                {data.data.map((a) => (
                  <tr key={a.id} className="hover:bg-surface-2">
                    <Td className="font-medium text-ink">{requestTypes[a.request.requestType]}</Td>
                    <Td>{a.request.requestedAddress ? <Mono>{a.request.requestedAddress}</Mono> : <span className="text-ink-3">Automatic</span>}</Td>
                    <Td className="whitespace-nowrap text-ink-2">{a.request.subnetName} <Mono>{a.request.subnetCidr}</Mono></Td>
                    <Td className="text-ink-2">{a.request.requestedByEmail}</Td>
                    <Td className="text-ink-2">{a.stepOrder} of {a.request.approvals.length}{a.approverRoleName && ` · ${a.approverRoleName}`}</Td>
                    <Td className="whitespace-nowrap text-ink-2">{a.request.submittedAt && formatDateTime(a.request.submittedAt)}</Td>
                    <Td className="text-right"><Button variant="primary" className="h-7" onClick={() => onOpen(a.request.id)}>Review</Button></Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
        </>
      )}
    </Card>
  )
}

function AllocationsTable({ onOpen, onRelease }: { onOpen: (id: string) => void; onRelease: (a: Allocation) => void }) {
  const canRequest = useCan('ip:request')
  const paging = usePaging('alloc')
  const [status, setStatus] = useState('active')
  const [search, setSearch] = useState('')
  const q = useDebounced(search.trim())
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, status, q })
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['allocations', qs], placeholderData: keepPreviousData, queryFn: () => list<Allocation>(`/allocations${qs}`),
  })
  useClampPage(paging, data?.meta)
  return (
    <Card>
      <div className="flex flex-wrap items-center gap-2 border-b border-line p-2.5">
        <div className="relative min-w-52 flex-1">
          <Search size={14} className="absolute top-1/2 left-2.5 -translate-y-1/2 text-ink-3" aria-hidden />
          <Input aria-label="Search allocations" placeholder="Address prefix or subnet name" className="pl-8" value={search} onChange={(e) => { setSearch(e.target.value); paging.setPage(1) }} />
        </div>
        <Select aria-label="Status" className="w-36!" value={status} onChange={(e) => { setStatus(e.target.value); paging.setPage(1) }}>
          <option value="active">Active</option><option value="released">Released</option><option value="all">All</option>
        </Select>
      </div>
      {isPending && <SkeletonRows rows={6} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && <EmptyState title="No allocations here" body="Approved allocation requests appear here once fulfilled." />}
      {data && data.data.length > 0 && (
        <>
          <div className="overflow-x-auto">
            <table className="w-full border-collapse">
              <thead><tr><Th>Address</Th><Th>Subnet</Th><Th>Routing domain</Th><Th>Owner</Th><Th>Allocated</Th><Th>Status</Th><Th /></tr></thead>
              <tbody>
                {data.data.map((a) => (
                  <tr key={a.id} className="hover:bg-surface-2">
                    <Td><Mono>{a.address}</Mono>{a.hostname && <span className="ml-2 text-ink-2">{a.hostname}</span>}</Td>
                    <Td className="whitespace-nowrap text-ink-2">{a.subnetName} <Mono>{a.subnetCidr}</Mono></Td>
                    <Td className="text-ink-2">{a.routingDomainName}</Td>
                    <Td className="text-ink-2">{a.ownerEmail ?? '—'}</Td>
                    <Td className="whitespace-nowrap text-ink-2">{formatDateTime(a.allocatedAt)}</Td>
                    <Td><Badge tone={a.status === 'active' ? 'accent' : 'neutral'}>{a.status === 'active' ? 'Allocated' : 'Released'}</Badge></Td>
                    <Td className="text-right whitespace-nowrap">
                      {a.requestId && <Button variant="ghost" className="h-7" onClick={() => onOpen(a.requestId!)}>Request</Button>}
                      {a.status === 'active' && (a.openReleaseRequestId
                        ? <Button variant="ghost" className="h-7" onClick={() => onOpen(a.openReleaseRequestId!)}>Release in progress</Button>
                        : canRequest && <Button className="h-7" onClick={() => onRelease(a)}>Request release</Button>)}
                    </Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
        </>
      )}
    </Card>
  )
}

const allSensitivities = ['standard', 'sensitive', 'restricted']

function toggle(list: string[], v: string) { return list.includes(v) ? list.filter((x) => x !== v) : [...list, v] }

function Checks({ legend, options, value, onChange }: { legend: string; options: Record<string, string>; value: string[]; onChange: (v: string[]) => void }) {
  return (
    <fieldset>
      <legend className="mb-1 text-[13px] font-medium text-ink">{legend} <span className="font-normal text-ink-3">(none ticked means any)</span></legend>
      <div className="flex flex-wrap gap-x-4 gap-y-1">
        {Object.entries(options).map(([k, label]) => (
          <label key={k} className="flex items-center gap-1.5 text-[13px] text-ink-2">
            <input type="checkbox" checked={value.includes(k)} onChange={() => onChange(toggle(value, k))} /> {label}
          </label>
        ))}
      </div>
    </fieldset>
  )
}

function WorkflowModal({ workflow, open, onClose }: { workflow: Workflow | null; open: boolean; onClose: () => void }) {
  const refresh = useRefresh()
  const queryClient = useQueryClient()
  const roles = useQuery({ queryKey: ['roles'], enabled: open, queryFn: () => listAll<Role>('/roles') })
  const [name, setName] = useState(workflow?.name ?? '')
  const [types, setTypes] = useState<string[]>(workflow?.matchRules.requestTypes ?? [])
  const [envs, setEnvs] = useState<string[]>(workflow?.matchRules.environments ?? [])
  const [sens, setSens] = useState<string[]>(workflow?.matchRules.sensitivities ?? [])
  const [active, setActive] = useState(workflow?.isActive ?? true)
  const [steps, setSteps] = useState<string[]>(workflow?.steps.map((s) => s.approverRoleId) ?? [''])
  const save = useMutation({
    mutationFn: () => {
      const body = { name, isActive: active, matchRules: { requestTypes: types, environments: envs, sensitivities: sens }, steps: steps.map((approverRoleId) => ({ approverRoleId })) }
      return workflow ? put(`/approval-workflows/${workflow.id}`, body) : post('/approval-workflows', body)
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['workflows'] }); refresh(); onClose() },
  })
  const errors = fieldErrors(save.error)
  return (
    <Modal open={open} onClose={onClose} title={workflow ? 'Edit workflow' : 'New approval workflow'} size="lg"
      subtitle="The most specific matching workflow applies when a request is submitted. Requests already submitted keep their steps."
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => save.mutate()}>Save workflow</Button></>}>
      <div className="space-y-4">
        <Field label="Name" htmlFor="wf-name" error={errors.name}><Input id="wf-name" value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Checks legend="Request types" options={requestTypes} value={types} onChange={setTypes} />
        <Checks legend="Environments" options={environments} value={envs} onChange={setEnvs} />
        <Checks legend="Subnet sensitivity" options={Object.fromEntries(allSensitivities.map((s) => [s, s.charAt(0).toUpperCase() + s.slice(1)]))} value={sens} onChange={setSens} />
        <fieldset>
          <legend className="mb-1 text-[13px] font-medium text-ink">Approval steps, in order</legend>
          <div className="space-y-1.5">
            {steps.map((role, i) => (
              <div key={i} className="flex items-center gap-2">
                <span className="w-14 text-[13px] text-ink-2">Step {i + 1}</span>
                <Select aria-label={`Step ${i + 1} approver role`} value={role} onChange={(e) => setSteps(steps.map((r, j) => (j === i ? e.target.value : r)))}>
                  <option value="">Choose the approver role</option>
                  {roles.data?.map((r) => <option key={r.id} value={r.id}>{r.name}</option>)}
                </Select>
                {steps.length > 1 && <Button variant="ghost" className="h-7 px-1.5" aria-label={`Remove step ${i + 1}`} onClick={() => setSteps(steps.filter((_, j) => j !== i))}><X size={14} /></Button>}
              </div>
            ))}
          </div>
          {steps.length < 5 && <Button variant="ghost" className="mt-1.5 h-7" onClick={() => setSteps([...steps, ''])}><Plus size={14} aria-hidden /> Add step</Button>}
          {errors.steps && <p role="alert" className="mt-1 text-[13px] text-danger">{errors.steps}</p>}
          <p className="mt-1.5 text-[12.5px] text-ink-3">Approvers also need the approve permission. Each step needs a different person, and nobody approves their own request.</p>
        </fieldset>
        <label className="flex items-center gap-2 text-[13px] text-ink"><input type="checkbox" checked={active} onChange={(e) => setActive(e.target.checked)} /> Active</label>
        {Object.keys(errors).length === 0 && <FormError error={save.error} />}
      </div>
    </Modal>
  )
}

function WorkflowsPanel() {
  const canManage = useCan('users:manage')
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<Workflow | null | 'new'>(null)
  const [removing, setRemoving] = useState<Workflow | null>(null)
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['workflows'], queryFn: () => list<Workflow>('/approval-workflows') })
  const remove = useMutation({
    mutationFn: (id: string) => del(`/approval-workflows/${id}`),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['workflows'] }); setRemoving(null) },
  })
  const rules = (w: Workflow) => {
    const parts = [
      w.matchRules.requestTypes?.map((t) => requestTypes[t]).join(', '),
      w.matchRules.environments?.map((e) => environments[e]).join(', '),
      w.matchRules.sensitivities?.join(', '),
    ].filter(Boolean)
    return parts.length ? parts.join(' · ') : 'Every request'
  }
  return (
    <Card title="Approval workflows" action={canManage && <Button variant="primary" className="h-7" onClick={() => setEditing('new')}><Plus size={14} aria-hidden /> New workflow</Button>}>
      <p className="border-b border-line px-4 py-2 text-[12.5px] text-ink-2">Without a matching workflow, a request needs one approval from anyone with the approve permission.</p>
      {isPending && <SkeletonRows rows={3} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && <EmptyState title="No workflows yet" body="Add one to require particular roles, or several levels, for some requests." />}
      {data && data.data.length > 0 && (
        <table className="w-full border-collapse">
          <thead><tr><Th>Name</Th><Th>Applies to</Th><Th>Steps</Th><Th>Used by</Th><Th>Status</Th>{canManage && <Th />}</tr></thead>
          <tbody>
            {data.data.map((w) => (
              <tr key={w.id}>
                <Td className="font-medium text-ink">{w.name}</Td>
                <Td className="text-ink-2">{rules(w)}</Td>
                <Td className="text-ink-2">{w.steps.map((s) => s.approverRoleName).join(' → ')}</Td>
                <Td className="text-ink-2 tabular-nums">{w.inUse} requests</Td>
                <Td><Badge tone={w.isActive ? 'ok' : 'neutral'}>{w.isActive ? 'Active' : 'Inactive'}</Badge></Td>
                {canManage && (
                  <Td className="text-right whitespace-nowrap">
                    <Button variant="ghost" className="h-7" onClick={() => setEditing(w)}>Edit</Button>
                    {w.inUse === 0 && <Button variant="ghost" className="h-7" onClick={() => setRemoving(w)}>Delete</Button>}
                  </Td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {editing && <WorkflowModal key={editing === 'new' ? 'new' : editing.id} open workflow={editing === 'new' ? null : editing} onClose={() => setEditing(null)} />}
      <ConfirmDialog open={removing !== null} title="Delete this workflow?" body="No request has used it, so nothing else changes."
        confirmLabel="Delete" busy={remove.isPending} error={remove.error} onConfirm={() => removing && remove.mutate(removing.id)} onCancel={() => setRemoving(null)} />
    </Card>
  )
}

export function Requests() {
  const canRequest = useCan('ip:request')
  const canApprove = useCan('requests:approve')
  const canAllocate = useCan('ip:allocate')
  const canReserve = useCan('ip:reserve')
  const canAudit = useCan('audit:view')
  const seeAll = canApprove || canAllocate || canReserve || canAudit
  const canManage = useCan('users:manage')
  const [params, setParams] = useSearchParams()
  const [creating, setCreating] = useState(false)
  const [releasing, setReleasing] = useState<Allocation | null>(null)
  const openId = params.get('id')
  const tab = params.get('tab') ?? (canApprove ? 'approvals' : 'mine')
  const pending = useQuery({ queryKey: ['approvals', 'count'], enabled: canApprove, queryFn: () => list<PendingApproval>('/approvals?pageSize=1') })

  const setParam = (key: string, value: string | null) => setParams((prev) => {
    const next = new URLSearchParams(prev)
    if (value) next.set(key, value)
    else next.delete(key)
    return next
  })
  const tabs: TabDef[] = [
    ...(canApprove ? [{ key: 'approvals', label: 'Waiting for me', count: pending.data?.meta.total }] : []),
    ...(canRequest ? [{ key: 'mine', label: 'My requests' }] : []),
    ...(seeAll ? [{ key: 'all', label: 'All requests' }] : []),
    { key: 'allocations', label: 'Allocations' },
    ...(canApprove || canManage ? [{ key: 'workflows', label: 'Workflows' }] : []),
  ]
  const active = tabs.some((t) => t.key === tab) ? tab : tabs[0].key

  return (
    <>
      <PageHeader title="IP Allocation Requests" description="Request, approve and fulfil allocations, reservations and releases. Every step is approved by someone else and audited."
        actions={canRequest && <Button variant="primary" onClick={() => setCreating(true)}><Plus size={15} aria-hidden /> New request</Button>} />
      <Tabs tabs={tabs} active={active} onChange={(key) => setParam('tab', key)} />
      {active === 'approvals' && <ApprovalsTable onOpen={(id) => setParam('id', id)} />}
      {active === 'mine' && <RequestsTable mine onOpen={(id) => setParam('id', id)} />}
      {active === 'all' && <RequestsTable mine={false} onOpen={(id) => setParam('id', id)} />}
      {active === 'allocations' && <AllocationsTable onOpen={(id) => setParam('id', id)} onRelease={setReleasing} />}
      {active === 'workflows' && <WorkflowsPanel />}
      {creating && <NewRequestModal open onClose={() => setCreating(false)} />}
      {releasing && <NewRequestModal open preset={{ requestType: 'release', allocation: releasing }} onClose={() => setReleasing(null)} />}
      <RequestModal id={openId} onClose={() => setParam('id', null)} />
    </>
  )
}

