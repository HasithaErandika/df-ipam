import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { changeableTypes, ProposeChange } from './ProposeChange'
import { Button, Field, FormError, Input, Modal, Mono, Select } from '../../components/ui'
import { post } from '../../lib/api'
import { classifications, formatDateTime } from '../../lib/format'
import { useCan } from '../../lib/session'
import type { Finding } from '../../lib/types'

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex justify-between gap-4 border-b border-line py-1.5 text-[13px] last:border-0">
      <dt className="text-ink-2">{label}</dt>
      <dd className="text-right text-ink">{value}</dd>
    </div>
  )
}

export function FindingModal({ finding, onClose }: { finding: Finding | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const canReview = useCan('inventory:edit')
  const [note, setNote] = useState('')
  const [status, setStatus] = useState(finding && finding.reviewStatus !== 'open' ? finding.reviewStatus : 'investigating')
  const review = useMutation({ meta: { success: 'Finding updated.', inlineError: true },
    mutationFn: () => post<void>(`/reconciliation/results/${finding?.id}/review`, { status, note }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['findings'] })
      queryClient.invalidateQueries({ queryKey: ['summary'] })
      setNote('')
      onClose()
    },
  })
  if (!finding) return null
  return (
    <Modal open onClose={onClose} title={classifications[finding.classification] ?? finding.classification} size="lg"
      subtitle={finding.address ? <Mono>{finding.address}</Mono> : finding.objectType.replaceAll('_', ' ')}
      footer={canReview ? <><Button onClick={onClose}>Close</Button><Button variant="primary" busy={review.isPending} onClick={() => review.mutate()}>Save review</Button></> : undefined}>
      <p className="text-ink">{finding.explanation}</p>
      <p className="mt-2 rounded-[5px] bg-accent-soft px-3 py-2 text-[13px] text-accent">{finding.suggestedAction}</p>
      <dl className="mt-4">
        <Row label="Routing domain" value={finding.routingDomainName ?? 'Not applicable'} />
        <Row label="Sources" value={finding.sources.join(', ')} />
        <Row label="Confidence" value={finding.confidence === null ? 'Unknown' : `${finding.confidence}%`} />
        <Row label="Source reliability" value={finding.sourceReliability === null ? 'Unknown' : `${finding.sourceReliability}%`} />
        {finding.confidenceBreakdown.reliability !== undefined && (
          <Row label="How the score was built" value={`Reliability ${finding.confidenceBreakdown.reliability}, freshness ${finding.confidenceBreakdown.freshness}, agreement ${finding.confidenceBreakdown.agreement}, capped at ${finding.confidenceBreakdown.cap}`} />
        )}
        {finding.evidenceTo && <Row label="Latest evidence" value={formatDateTime(finding.evidenceTo)} />}
        <Row label="Last detected" value={formatDateTime(finding.lastDetectedAt)} />
        {finding.reviewedBy && <Row label="Reviewed by" value={finding.reviewedBy} />}
        {finding.reviewNote && <Row label="Review note" value={finding.reviewNote} />}
      </dl>
      {canReview && (
        <div className="mt-4 grid grid-cols-[11rem_1fr] gap-3">
          <Field label="Review status" htmlFor="review-status">
            <Select id="review-status" value={status} onChange={(e) => setStatus(e.target.value)}>
              <option value="investigating">Investigating</option>
              <option value="accepted">Accepted</option>
              <option value="dismissed">Dismissed</option>
              <option value="resolved">Resolved</option>
            </Select>
          </Field>
          <Field label="Note" htmlFor="review-note"><Input id="review-note" value={note} onChange={(e) => setNote(e.target.value)} /></Field>
        </div>
      )}
      <div className="mt-3"><FormError error={review.error} /></div>
      {canReview && finding.objectId && changeableTypes.includes(finding.objectType) && <ProposeChange finding={finding} onDone={onClose} />}
      <p className="mt-3 text-[12px] text-ink-3">Reviewing a finding never changes inventory. A proposed change takes effect only when a second person approves it.</p>
    </Modal>
  )
}
