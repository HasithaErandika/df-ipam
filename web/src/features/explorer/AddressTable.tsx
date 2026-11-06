import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Search } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { allocationStates, DeviceIcon, deviceTypes, observationStates, OSBadge, StateBadges } from '../../components/devices'
import { SavedFilters } from '../../components/SavedFilters'
import { EmptyState, ErrorState, Input, Mono, Pagination, Select, SkeletonRows, Td, Th } from '../../components/ui'
import { useDebounced } from '../../components/workflow'
import { list, queryString } from '../../lib/api'
import { formatDateTime } from '../../lib/format'
import { useClampPage, usePaging } from '../../lib/paging'
import { useMe } from '../../lib/session'
import type { IPRecord } from '../../lib/types'

export type SetParams = (updates: Record<string, string | null>) => void

function AddressRow({ record: r, full, onOpen }: { record: IPRecord; full: boolean; onOpen: () => void }) {
  return (
    <tr className="cursor-pointer hover:bg-surface-2" onClick={onOpen}>
      <Td><button type="button" className="cursor-pointer hover:text-accent"><Mono>{r.address}</Mono></button></Td>
      {full && <Td>
        <span className="flex items-center gap-2">
          <DeviceIcon type={r.deviceType} />
          <span className="min-w-0">
            <span className="block truncate text-ink">{r.deviceName || r.hostname || '—'}</span>
            {r.deviceType && <span className="block text-[11.5px] text-ink-3">{deviceTypes[r.deviceType]?.label}{r.vendor && ` · ${r.vendor}`}</span>}
          </span>
        </span>
      </Td>}
      {!full && <Td className="text-ink">{r.hostname || '—'}</Td>}
      {full && <Td><OSBadge family={r.osFamily} version={r.operatingSystem} /></Td>}
      {full && <Td>{r.mac ? <Mono>{r.mac}</Mono> : <span className="text-ink-3">—</span>}</Td>}
      <Td className="whitespace-nowrap text-ink-2">{r.subnetName} <span className="text-ink-3">· {r.routingDomainName}</span></Td>
      <Td className="text-ink-2">{r.ownerEmail ?? '—'}</Td>
      <Td><StateBadges allocation={r.allocationState} observation={r.observationState} /></Td>
      <Td className="whitespace-nowrap text-ink-2">{r.lastSeen ? formatDateTime(r.lastSeen) : '—'}</Td>
    </tr>
  )
}

/** Tracked addresses with their filters, across everything or within one subnet. */
export function AddressTable({ params, set, subnetId, toolbarEnd }:
  { params: URLSearchParams; set: SetParams; subnetId: string; toolbarEnd?: ReactNode }) {
  const me = useMe()
  const full = me.permissions.includes('inventory:view')
  const assignable = me.permissions.includes('inventory:edit_assigned') && !me.permissions.includes('inventory:edit')
  const paging = usePaging()
  const mine = params.get('mine') === '1'
  const allocationState = params.get('allocationState') ?? ''
  const observationState = params.get('observationState') ?? ''
  const [search, setSearch] = useState(params.get('q') ?? '')
  const q = useDebounced(search.trim())
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, subnetId, allocationState, observationState, q, ownerId: mine ? me.id : undefined })
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['ip', 'list', qs], placeholderData: keepPreviousData, queryFn: () => list<IPRecord>(`/ip-addresses${qs}`) })
  useClampPage(paging, data?.meta)
  const filtered = Boolean(subnetId || allocationState || observationState || q || mine)
  return (
    <>
      <div className="flex flex-wrap items-center gap-2 border-b border-line p-2.5">
        <div className="relative min-w-52 flex-1">
          <Search size={14} className="absolute top-1/2 left-2.5 -translate-y-1/2 text-ink-3" aria-hidden />
          <Input aria-label="Search addresses" placeholder={full ? 'Address, CIDR, hostname, FQDN or MAC' : 'Address, CIDR, hostname or FQDN'} className="pl-8" value={search} onChange={(e) => { setSearch(e.target.value); paging.setPage(1) }} />
        </div>
        <Select aria-label="Allocation state" className="w-44!" value={allocationState} onChange={(e) => set({ allocationState: e.target.value })}>
          <option value="">Any allocation</option>
          {Object.entries(allocationStates).map(([k, v]) => <option key={k} value={k}>{v.label}</option>)}
        </Select>
        <Select aria-label="Observation state" className="w-44!" value={observationState} onChange={(e) => set({ observationState: e.target.value })}>
          <option value="">Any observation</option>
          {Object.entries(observationStates).map(([k, v]) => <option key={k} value={k}>{v.label}</option>)}
        </Select>
        {assignable && <label className="flex items-center gap-1.5 text-[13px] text-ink-2"><input type="checkbox" className="size-4 accent-[var(--accent)]" checked={mine} onChange={(e) => set({ mine: e.target.checked ? '1' : null })} /> Assigned to me</label>}
        <SavedFilters page="ip-addresses" current={{ subnetId, allocationState, observationState, q }}
          onApply={(f) => { setSearch(f.q ?? ''); set({ subnetId: f.subnetId ?? null, allocationState: f.allocationState ?? null, observationState: f.observationState ?? null }) }} />
        {toolbarEnd}
      </div>
      {isPending && <SkeletonRows rows={8} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && (filtered
        ? <EmptyState title="No addresses match" body={subnetId ? 'Only addresses of interest have records. Switch to the grid to see every address in the subnet.' : undefined} />
        : <EmptyState title="No IP records yet" body="Records appear as addresses are allocated, imported or reported by approved sources." />)}
      {data && data.data.length > 0 && (
        <>
          <div className="overflow-x-auto">
            <table className="w-full border-collapse">
              <thead><tr><Th>Address</Th>{full ? <><Th>Device</Th><Th>OS</Th><Th>MAC</Th></> : <Th>Hostname</Th>}<Th>Subnet</Th><Th>Owner</Th><Th>State</Th><Th>Last seen</Th></tr></thead>
              <tbody>{data.data.map((r) => <AddressRow key={r.id} record={r} full={full} onOpen={() => set({ id: r.id })} />)}</tbody>
            </table>
          </div>
          <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
        </>
      )}
    </>
  )
}
