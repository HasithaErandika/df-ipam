import { useMutation, useQuery } from '@tanstack/react-query'
import { Sparkles } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Button, Details, Field, FormError, Input, Modal, Textarea } from '../../components/ui'
import { get, post, queryString } from '../../lib/api'
import { formatDate } from '../../lib/format'
import { ipTypes, label } from '../../lib/labels'
import { inventoryKeys, useInvalidate } from '../../lib/queries'
import type { IPRequest } from '../../lib/types'
import { isIPv4 } from '../../lib/validation'
import { AddressCheck } from '../shared/AddressCheck'

/** Approves with one address: the requested one, or the Available candidate. */
export function ApproveRequestModal({ request, onClose }: { request: IPRequest | null; onClose: () => void }) {
  const [ip, setIp] = useState('')
  const refresh = useInvalidate(...inventoryKeys)
  const candidate = useQuery({
    queryKey: ['/ip-addresses/available-candidate', request?.segmentId],
    queryFn: () => get<{ ip: string }>(`/ip-addresses/available-candidate${queryString({ segmentId: request!.segmentId })}`),
    enabled: Boolean(request && !request.requestedIp), retry: false,
  })
  useEffect(() => { setIp(request?.requestedIp ?? '') }, [request])
  useEffect(() => { if (candidate.data && request && !request.requestedIp) setIp((v) => v || candidate.data.ip) }, [candidate.data, request])
  const approve = useMutation({
    mutationFn: () => post<{ ip: string }>(`/ip-requests/${request!.id}/decision`, { status: 'approved', ip }),
    meta: { inlineError: true, success: (_: void, r: { ip: string }) => `Approved. ${r.ip} is now in the register and the requester has been told.` },
    onSuccess: () => { void refresh(); onClose() },
  })
  return (
    <Modal open={request !== null} onClose={onClose} title="Approve request" subtitle={request?.name} size="lg"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={approve.isPending} disabled={!isIPv4(ip.trim())} onClick={() => approve.mutate()}>Approve with {ip || 'this IP'}</Button></>}>
      {request && (
        <div className="space-y-4">
          <Details items={[
            ['Requested by', request.requesterEmail], ['Segment', request.segmentName], ['Type', label(ipTypes, request.ipType)],
            ['Environment', request.environmentName ?? 'Segment default'], ['Needed by', request.neededBy ? formatDate(request.neededBy) : 'Not given'],
            ['Purpose', request.purpose],
          ]} />
          <Field label="IP to assign" htmlFor="ap-ip" hint={request.requestedIp ? 'The requester asked for this address.' : 'The next Available candidate. You can type another.'}>
            <div className="flex gap-2">
              <Input id="ap-ip" className="font-mono" value={ip} onChange={(e) => setIp(e.target.value)} />
              <Button busy={candidate.isFetching} onClick={() => void candidate.refetch().then((r) => r.data && setIp(r.data.ip))}><Sparkles size={14} aria-hidden />Suggest</Button>
            </div>
          </Field>
          <AddressCheck segmentId={request.segmentId} ip={ip} allowSeen={false} />
          <FormError error={approve.error ?? candidate.error} />
        </div>
      )}
    </Modal>
  )
}

/** Rejects with a reason the requester will read. */
export function RejectRequestModal({ request, onClose }: { request: IPRequest | null; onClose: () => void }) {
  const [reason, setReason] = useState('')
  const refresh = useInvalidate('/ip-requests', '/dashboard')
  useEffect(() => { setReason('') }, [request])
  const reject = useMutation({
    mutationFn: () => post(`/ip-requests/${request!.id}/decision`, { status: 'rejected', reason }),
    meta: { inlineError: true, success: 'Request rejected. The requester has been told why.' },
    onSuccess: () => { void refresh(); onClose() },
  })
  return (
    <Modal open={request !== null} onClose={onClose} title="Reject request" subtitle={request?.name}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="danger" busy={reject.isPending} disabled={!reason.trim()} onClick={() => reject.mutate()}>Reject request</Button></>}>
      <div className="space-y-3">
        <Field label="Reason" htmlFor="rj-reason" hint="Sent to the requester." error={reason.length > 500 ? 'Keep the reason under 500 characters.' : undefined}>
          <Textarea id="rj-reason" value={reason} autoFocus onChange={(e) => setReason(e.target.value)} />
        </Field>
        <FormError error={reject.error} />
      </div>
    </Modal>
  )
}
