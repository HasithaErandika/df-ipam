import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Badge, Button, Card, EmptyState, ErrorState, Field, FormError, Input, Modal, Pagination, Select, SkeletonRows, Td, Th } from '../../components/ui'
import { list, post, queryString } from '../../lib/api'
import { formatDateTime } from '../../lib/format'
import { useClampPage, usePaging } from '../../lib/paging'
import { useCan, useMe } from '../../lib/session'
import type { ChangeRequest } from '../../lib/types'
import { label } from './ProposeChange'

const tones = { pending: 'warn', approved: 'accent', applied: 'ok', rejected: 'neutral' } as const

function Decision({ change, onClose }: { change: ChangeRequest | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const me = useMe()
  const canDecide = useCan('requests:approve')
  const [note, setNote] = useState('')
  const [noteMissing, setNoteMissing] = useState(false)
  const decide = useMutation({ meta: { success: (a: string) => a === 'approve' ? 'Change approved and applied.' : 'Change rejected.', inlineError: true },
    mutationFn: (action: 'approve' | 'reject') => post(`/change-requests/${change?.id}/${action}`, { note }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['changes'] })
      queryClient.invalidateQueries({ queryKey: ['findings'] })
      setNote('')
      onClose()
    },
  })
  if (!change) return null
  const own = change.requestedById === me.id
  const open = change.status === 'pending' && canDecide && !own
  return (
    <Modal open onClose={onClose} title="Change request" subtitle={`${change.objectType.replaceAll('_', ' ')} · ${change.requestedBy ?? 'Unknown'}`} size="lg"
      footer={open ? <><Button onClick={onClose}>Close</Button><Button busy={decide.isPending} onClick={() => (note.trim() ? decide.mutate('reject') : setNoteMissing(true))}>Reject</Button><Button variant="primary" busy={decide.isPending} onClick={() => decide.mutate('approve')}>Approve and apply</Button></> : undefined}>
      {change.finding && <p className="text-ink">{change.finding}</p>}
      <p className="mt-2 text-[13px] text-ink-2">Reason: {change.reason}</p>
      <table className="mt-3 w-full border-collapse">
        <thead><tr><Th>Field</Th><Th>Proposed value</Th></tr></thead>
        <tbody>{Object.entries(change.proposed).map(([k, v]) => <tr key={k}><Td className="text-ink-2">{label(k)}</Td><Td>{v || 'Cleared'}</Td></tr>)}</tbody>
      </table>
      {change.decidedBy && <p className="mt-3 text-[13px] text-ink-2">Decided by {change.decidedBy}{change.decisionNote ? `: ${change.decisionNote}` : ''}</p>}
      {own && change.status === 'pending' && <p className="mt-3 text-[13px] text-ink-2">Someone else must decide a change you proposed.</p>}
      {open && <div className="mt-3"><Field label="Decision note (needed to reject)" htmlFor="change-note" error={noteMissing && !note.trim() ? 'Say why this change is rejected.' : undefined}><Input id="change-note" value={note} onChange={(e) => setNote(e.target.value)} /></Field></div>}
      <div className="mt-2"><FormError error={decide.error} /></div>
    </Modal>
  )
}

export function ChangeRequests() {
  const paging = usePaging()
  const [status, setStatus] = useState('pending')
  const [selected, setSelected] = useState<ChangeRequest | null>(null)
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, status })
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['changes', qs], placeholderData: keepPreviousData, queryFn: () => list<ChangeRequest>(`/change-requests${qs}`) })
  useClampPage(paging, data?.meta)
  return (
    <Card>
      <div className="border-b border-line p-2.5">
        <Select aria-label="Status" className="w-44!" value={status} onChange={(e) => { setStatus(e.target.value); paging.setPage(1) }}>
          <option value="pending">Waiting for a decision</option><option value="applied">Applied</option><option value="rejected">Rejected</option><option value="">All</option>
        </Select>
      </div>
      {isPending && <SkeletonRows rows={6} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && <EmptyState title="No change requests here" body="Propose one from a finding in the Findings tab." />}
      {data && data.data.length > 0 && (
        <>
          <div className="overflow-x-auto">
            <table className="w-full border-collapse">
              <thead><tr><Th>Record</Th><Th>Changes</Th><Th>Proposed by</Th><Th>Proposed</Th><Th>Status</Th></tr></thead>
              <tbody>
                {data.data.map((c) => (
                  <tr key={c.id} className="hover:bg-surface-2">
                    <Td><button type="button" className="cursor-pointer font-medium text-ink hover:text-accent" onClick={() => setSelected(c)}>{c.objectType.replaceAll('_', ' ')}</button></Td>
                    <Td className="text-ink-2">{Object.keys(c.proposed).map(label).join(', ')}</Td>
                    <Td className="text-ink-2">{c.requestedBy ?? '—'}</Td>
                    <Td className="whitespace-nowrap text-ink-2">{formatDateTime(c.createdAt)}</Td>
                    <Td><Badge tone={tones[c.status]}>{c.status}</Badge></Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
        </>
      )}
      <Decision change={selected} onClose={() => setSelected(null)} />
    </Card>
  )
}
