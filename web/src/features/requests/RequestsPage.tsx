import { useMutation } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useState } from 'react'
import { Badge, Button, Card, DataTable, FilterBar, FilterSelect, none, PageHeader, PathTabs, SearchInput, Stack, useConfirm, type Column } from '../../components/ui'
import { post } from '../../lib/api'
import { formatAgo, formatDate, formatDateTime } from '../../lib/format'
import { ipTypes, label, options, requestStatuses, requestTone } from '../../lib/labels'
import { useFilters, usePagedList } from '../../lib/listing'
import { useInvalidate } from '../../lib/queries'
import { useCan, useMe } from '../../lib/session'
import { useRouteTab } from '../../lib/tabs'
import type { IPRequest } from '../../lib/types'
import { ApproveRequestModal, RejectRequestModal } from './DecisionModals'
import { RequestForm } from './RequestForm'

export function RequestsPage() {
  const me = useMe()
  const canApprove = useCan('requests:approve')
  const canAsk = useCan('ip:request')
  const tab = useRouteTab(['requests', 'all'])
  const f = useFilters(['status', 'q'] as const)
  const status = canApprove && tab === 'requests' ? 'pending' : f.values.status
  const { query, rows, meta, paging } = usePagedList<IPRequest>('/ip-requests', { status, q: f.values.q })
  const [creating, setCreating] = useState(false)
  const [approving, setApproving] = useState<IPRequest | null>(null)
  const [rejecting, setRejecting] = useState<IPRequest | null>(null)
  const confirm = useConfirm()
  const refresh = useInvalidate('/ip-requests', '/dashboard')
  const cancel = useMutation({ mutationFn: (id: string) => post(`/ip-requests/${id}/cancel`), meta: { success: 'Request cancelled.' }, onSuccess: () => void refresh() })

  const columns: Column<IPRequest>[] = [
    { key: 'name', header: 'Request', cell: (r) => <Stack main={<span className="font-medium">{r.name}</span>} sub={`${label(ipTypes, r.ipType)} · ${r.segmentName}`} /> },
    { key: 'purpose', header: 'Purpose', hideBelow: 'lg', className: 'max-w-xs', cell: (r) => <span className="line-clamp-2 text-ink-2">{r.purpose || '—'}</span> },
    { key: 'who', header: 'Requested', cell: (r) => <Stack main={r.requesterId === me.id ? 'You' : r.requesterEmail} sub={<span title={formatDateTime(r.createdAt)}>{formatAgo(r.createdAt)}</span>} /> },
    { key: 'needed', header: 'Needed by', hideBelow: 'md', cell: (r) => (r.neededBy ? formatDate(r.neededBy) : none) },
    { key: 'status', header: 'Status', cell: (r) => <Stack main={<Badge tone={requestTone[r.status]}>{label(requestStatuses, r.status)}</Badge>}
      sub={r.status === 'rejected' ? r.decisionReason : r.decidedByEmail ? `by ${r.decidedByEmail}` : undefined} /> },
    { key: 'ip', header: 'IP', cell: (r) => (r.assignedIp ? <span className="font-mono text-[12.5px] font-medium text-ok">{r.assignedIp}</span> : r.requestedIp ? <span className="font-mono text-[12.5px] text-ink-3" title="Requested">{r.requestedIp}?</span> : none) },
    { key: 'act', header: '', className: 'text-right whitespace-nowrap', cell: (r) => {
      if (r.status !== 'pending') return null
      if (canApprove) return <div className="flex justify-end gap-1"><Button onClick={() => setApproving(r)}>Approve</Button><Button variant="ghost" className="text-danger" onClick={() => setRejecting(r)}>Reject</Button></div>
      if (r.requesterId === me.id) {
        return <Button variant="ghost" onClick={async () => { if (await confirm({ title: `Cancel the request for ${r.name}?`, body: 'Admins will no longer see it in their queue.', confirmLabel: 'Cancel request' })) cancel.mutate(r.id) }}>Cancel</Button>
      }
      return null
    } },
  ]

  const tabs = canApprove ? [{ key: 'requests', label: 'Waiting for decision', count: tab === 'requests' ? meta?.total : undefined }, { key: 'all', label: 'All requests' }] : [{ key: 'requests', label: 'My requests' }]
  return (
    <>
      <PageHeader title="IP Requests" description={canApprove ? 'Approve with one address, or reject with a reason. Approval adds the address to the register in the same step.' : 'Ask for an IP for a new device. You are told the address once an Admin approves.'}
        actions={canAsk && <Button variant="primary" onClick={() => setCreating(true)}><Plus size={14} aria-hidden />Request an IP</Button>} />
      <PathTabs base="/requests" tabs={tabs} />
      <Card>
        <FilterBar active={f.active} onClear={f.clear}>
          <SearchInput value={f.values.q} onChange={(v) => f.set('q', v)} placeholder="Search name, purpose, requester" />
          {!(canApprove && tab === 'requests') && <FilterSelect label="Status" value={f.values.status} onChange={(v) => f.set('status', v)} anyLabel="Any status" options={options(requestStatuses)} />}
        </FilterBar>
        <DataTable columns={columns} rows={rows} query={query} rowKey={(r) => r.id} meta={meta} paging={paging} filtered={f}
          empty={canApprove && tab === 'requests' ? { title: 'Nothing waiting', body: 'New requests appear here, and Admins are notified.' } : { title: 'No requests yet', body: 'Raise a request when you need an IP for a new device.' }} />
      </Card>
      <RequestForm open={creating} onClose={() => setCreating(false)} />
      <ApproveRequestModal request={approving} onClose={() => setApproving(null)} />
      <RejectRequestModal request={rejecting} onClose={() => setRejecting(null)} />
    </>
  )
}
