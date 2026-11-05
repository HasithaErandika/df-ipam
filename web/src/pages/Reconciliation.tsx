import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Play } from 'lucide-react'
import { useState } from 'react'
import { useSearchParams } from 'react-router'
import { ChangeRequests, FindingRules, ProposeChange, changeableTypes } from '../components/changes'
import { Tabs } from '../components/workflow'
import { Badge, Button, Card, EmptyState, ErrorState, Field, FormError, Input, Modal, Mono, PageHeader, Pagination, Select, SkeletonRows, Td, Th } from '../components/ui'
import { list, get, post, queryString } from '../lib/api'
import { classifications, formatDateTime } from '../lib/format'
import { useClampPage, usePaging } from '../lib/paging'
import { useCan } from '../lib/session'
import type { Finding, FindingCount } from '../lib/types'

const statuses: Record<string, string> = {
  active: 'Needs review', investigating: 'Investigating', accepted: 'Accepted', dismissed: 'Dismissed', resolved: 'Resolved', all: 'All',
}

function statusTone(s: string) {
  if (s === 'open') return 'warn' as const
  if (s === 'investigating') return 'accent' as const
  return s === 'resolved' || s === 'accepted' ? 'ok' as const : 'neutral' as const
}

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex justify-between gap-4 border-b border-line py-1.5 text-[13px] last:border-0">
      <dt className="text-ink-2">{label}</dt>
      <dd className="text-right text-ink">{value}</dd>
    </div>
  )
}

function FindingModal({ finding, onClose }: { finding: Finding | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const canReview = useCan('inventory:edit')
  const [note, setNote] = useState('')
  const [status, setStatus] = useState('investigating')
  const review = useMutation({
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

const tabs = [{ key: 'findings', label: 'Findings' }, { key: 'changes', label: 'Change requests' }, { key: 'rules', label: 'Rules and confidence' }]

export function Reconciliation() {
  const [params, setParams] = useSearchParams()
  const tab = tabs.some((t) => t.key === params.get('tab')) ? params.get('tab')! : 'findings'
  return (
    <>
      <PageHeader title="Reconciliation Centre" description="Differences between inventory and evidence. Nothing here changes inventory until a person acts." />
      <Tabs tabs={tabs} active={tab} onChange={(key) => setParams({ tab: key })} />
      {tab === 'findings' && <Findings />}
      {tab === 'changes' && <ChangeRequests />}
      {tab === 'rules' && <FindingRules />}
    </>
  )
}

function Findings() {
  const queryClient = useQueryClient()
  const canRun = useCan('discovery:run')
  const [status, setStatus] = useState('active')
  const [classification, setClassification] = useState('')
  const paging = usePaging()
  const [selected, setSelected] = useState<Finding | null>(null)
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, status, classification })
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['findings', qs], placeholderData: keepPreviousData, queryFn: () => list<Finding>(`/reconciliation/results${qs}`),
  })
  useClampPage(paging, data?.meta)
  const summary = useQuery({ queryKey: ['findings', 'summary'], queryFn: () => get<FindingCount[]>('/reconciliation/summary') })
  const run = useMutation({
    mutationFn: () => post<{ detected: number; resolved: number }>('/reconciliation/run'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['findings'] })
      queryClient.invalidateQueries({ queryKey: ['summary'] })
    },
  })
  return (
    <>
      {canRun && <div className="mb-3 flex justify-end"><Button variant="primary" busy={run.isPending} onClick={() => run.mutate()}><Play size={14} aria-hidden /> Run now</Button></div>}
      {run.error ? <div className="mb-3"><FormError error={run.error} /></div> : null}
      {summary.data && summary.data.length > 0 && (
        <div className="mb-3 flex flex-wrap gap-1.5">
          {summary.data.map((s) => (
            <button key={s.classification} type="button" aria-pressed={classification === s.classification}
              onClick={() => { setClassification(classification === s.classification ? '' : s.classification); paging.setPage(1) }}
              className={`flex h-7 cursor-pointer items-center gap-1.5 rounded-[5px] border px-2 text-[12.5px] ${classification === s.classification ? 'border-accent bg-accent-soft text-accent' : 'border-line bg-surface text-ink-2 hover:border-line-strong'}`}>
              {classifications[s.classification] ?? s.classification}
              <span className="font-semibold tabular-nums">{s.count}</span>
            </button>
          ))}
        </div>
      )}
      <Card>
        <div className="flex items-center gap-2 border-b border-line p-2.5">
          <Select aria-label="Review status" className="w-44!" value={status} onChange={(e) => { setStatus(e.target.value); paging.setPage(1) }}>
            {Object.entries(statuses).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </Select>
        </div>
        {isPending && <SkeletonRows rows={8} />}
        {error && <ErrorState error={error} onRetry={refetch} />}
        {data?.data.length === 0 && <EmptyState title="No findings here" body="Run reconciliation to compare inventory with evidence." />}
        {data && data.data.length > 0 && (
          <>
            <div className="overflow-x-auto">
              <table className="w-full border-collapse">
                <thead><tr><Th>Finding</Th><Th>Detail</Th><Th>Routing domain</Th><Th className="text-right">Confidence</Th><Th>Status</Th></tr></thead>
                <tbody>
                  {data.data.map((f) => (
                    <tr key={f.id} className="hover:bg-surface-2">
                      <Td className="whitespace-nowrap"><button type="button" className="cursor-pointer font-medium text-ink hover:text-accent" onClick={() => setSelected(f)}>{classifications[f.classification] ?? f.classification}</button></Td>
                      <Td className="text-ink-2">{f.explanation}</Td>
                      <Td className="whitespace-nowrap text-ink-2">{f.routingDomainName ?? '—'}</Td>
                      <Td className="text-right text-ink-2 tabular-nums">{f.confidence === null ? '—' : `${f.confidence}%`}</Td>
                      <Td><Badge tone={statusTone(f.reviewStatus)}>{f.reviewStatus}</Badge></Td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
          </>
        )}
      </Card>
      <FindingModal finding={selected} onClose={() => setSelected(null)} />
    </>
  )
}
