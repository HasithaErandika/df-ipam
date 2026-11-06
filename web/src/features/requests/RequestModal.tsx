import { useMutation, useQuery } from '@tanstack/react-query'
import { Check, X } from 'lucide-react'
import { useState } from 'react'
import { Badge, Button, ConfirmDialog, ErrorState, Field, FormError, Input, Modal, Mono, SkeletonRows } from '../../components/ui'
import { Detail, StatusBadge } from '../../components/workflow'
import { del, fieldIssues, get, post } from '../../lib/api'
import { environments, formatDate, formatDateTime, formatEndOfDay, requestTypes } from '../../lib/format'
import { useCan, useMe } from '../../lib/session'
import type {  IPRequest } from '../../lib/types'
import { useRefresh } from './useRefresh'

function decisionTone(d: string) {
  return d === 'approved' ? 'ok' as const : d === 'rejected' ? 'danger' as const : 'neutral' as const
}

export function RequestModal({ id, onClose }: { id: string | null; onClose: () => void }) {
  const me = useMe()
  const refresh = useRefresh()
  const canAllocate = useCan('ip:allocate')
  const canReserve = useCan('ip:reserve')
  const [comment, setComment] = useState('')
  const [confirm, setConfirm] = useState<'cancel' | 'delete' | null>(null)
  const { data: r, isPending, error, refetch } = useQuery({
    queryKey: ['request', id], enabled: Boolean(id), queryFn: () => get<IPRequest>(`/requests/${id}`),
  })
  const act = useMutation({ meta: { success: (a: string) => ({ submit: 'Request submitted for approval.', cancel: 'Request cancelled.', delete: 'Draft deleted.', approve: 'Step approved.', reject: 'Request rejected.', fulfil: 'Request fulfilled.' })[a], inlineError: true },
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
  const errors = fieldIssues(act.error)

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
