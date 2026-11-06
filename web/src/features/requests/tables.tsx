import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Search } from 'lucide-react'
import { useState } from 'react'
import { Badge, Button, Card, EmptyState, ErrorState, Input, Mono, Pagination, Select, SkeletonRows, Td, Th } from '../../components/ui'
import { StatusBadge, useDebounced } from '../../components/workflow'
import { list, queryString } from '../../lib/api'
import { formatDateTime, requestStatuses, requestTypes } from '../../lib/format'
import { useClampPage, usePaging } from '../../lib/paging'
import { useCan } from '../../lib/session'
import type { Allocation, IPRequest, PendingApproval } from '../../lib/types'

export function RequestsTable({ mine, onOpen }: { mine: boolean; onOpen: (id: string) => void }) {
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

export function ApprovalsTable({ onOpen }: { onOpen: (id: string) => void }) {
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

export function AllocationsTable({ onOpen, onRelease }: { onOpen: (id: string) => void; onRelease: (a: Allocation) => void }) {
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
