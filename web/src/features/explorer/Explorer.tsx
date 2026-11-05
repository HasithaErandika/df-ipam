import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Grid3x3, List, Plus, Radar, Search } from 'lucide-react'
import { useState } from 'react'
import { useSearchParams } from 'react-router'
import { allocationStates, DeviceIcon, deviceTypes, observationStates, OSBadge, StateBadges } from '../../components/devices'
import { SavedFilters } from '../../components/SavedFilters'
import { Button, Card, EmptyState, ErrorState, Input, Mono, PageHeader, Pagination, Select, SkeletonRows, Td, Th } from '../../components/ui'
import { SubnetPicker, useDebounced } from '../../components/workflow'
import { StartJobModal } from '../discovery/StartJobModal'
import { NewRecord, RecordModal } from './records'
import { SubnetGrid } from './SubnetGrid'
import { get, list, queryString } from '../../lib/api'
import { formatDateTime } from '../../lib/format'
import { useClampPage, usePaging } from '../../lib/paging'
import { useCan } from '../../lib/session'
import type { IPRecord, Subnet } from '../../lib/types'

export function Explorer() {
  const canCreate = useCan('inventory:create')
  const canScan = useCan('discovery:run')
  const [scanning, setScanning] = useState(false)
  const [params, setParams] = useSearchParams()
  const paging = usePaging()
  const [creating, setCreating] = useState(false)
  const [subnet, setSubnet] = useState<Subnet | null>(null)
  const subnetId = params.get('subnetId') ?? ''
  const view = params.get('view') === 'grid' && subnetId ? 'grid' : 'table'
  const allocationState = params.get('allocationState') ?? ''
  const observationState = params.get('observationState') ?? ''
  const [search, setSearch] = useState(params.get('q') ?? '')
  const q = useDebounced(search.trim())
  const openId = params.get('id')
  const set = (updates: Record<string, string | null>) => setParams((prev) => {
    const next = new URLSearchParams(prev)
    for (const [k, v] of Object.entries(updates)) {
      if (v) next.set(k, v)
      else next.delete(k)
    }
    if (!('page' in updates) && !('id' in updates) && !('view' in updates)) next.delete('page')
    return next
  })
  const pinned = useQuery({ queryKey: ['subnet', subnetId], enabled: Boolean(subnetId) && subnet?.id !== subnetId, queryFn: () => get<Subnet>(`/subnets/${subnetId}`) })
  const current = subnet?.id === subnetId ? subnet : pinned.data ?? null
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, subnetId, allocationState, observationState, q })
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['ip', 'list', qs], placeholderData: keepPreviousData, enabled: view === 'table', queryFn: () => list<IPRecord>(`/ip-addresses${qs}`),
  })
  useClampPage(paging, data?.meta)
  const filtered = Boolean(subnetId || allocationState || observationState || q)
  return (
    <>
      <PageHeader title="IP Address Explorer" description="Every tracked address, the device behind it, and its history. Allocation and observation are always shown separately."
        actions={<>
          {canScan && <Button disabled={!current} title={current ? 'Discover this subnet under an approved policy' : 'Choose a subnet first'} onClick={() => setScanning(true)}><Radar size={15} aria-hidden /> Scan subnet</Button>}
          {canCreate && <Button variant="primary" onClick={() => setCreating(true)}><Plus size={15} aria-hidden /> New record</Button>}
        </>} />
      <Card>
        <div className="flex flex-wrap items-start gap-2 border-b border-line p-2.5">
          <div className="w-full max-w-md">
            <SubnetPicker id="ex-subnet" value={current} onChange={(s) => { setSubnet(s); set({ subnetId: s?.id ?? null, view: s ? view : null }) }} />
          </div>
          <div className="relative min-w-52 flex-1">
            <Search size={14} className="absolute top-1/2 left-2.5 -translate-y-1/2 text-ink-3" aria-hidden />
            <Input aria-label="Search addresses" placeholder="Address, hostname, FQDN or MAC" className="pl-8" value={search}
              onChange={(e) => { setSearch(e.target.value); paging.setPage(1) }} />
          </div>
          <Select aria-label="Allocation state" className="w-44!" value={allocationState} onChange={(e) => set({ allocationState: e.target.value })}>
            <option value="">Any allocation</option>
            {Object.entries(allocationStates).map(([k, v]) => <option key={k} value={k}>{v.label}</option>)}
          </Select>
          <Select aria-label="Observation state" className="w-44!" value={observationState} onChange={(e) => set({ observationState: e.target.value })}>
            <option value="">Any observation</option>
            {Object.entries(observationStates).map(([k, v]) => <option key={k} value={k}>{v.label}</option>)}
          </Select>
          <SavedFilters page="ip-addresses" current={{ subnetId, allocationState, observationState, q }}
            onApply={(f) => { setSearch(f.q ?? ''); set({ subnetId: f.subnetId ?? null, allocationState: f.allocationState ?? null, observationState: f.observationState ?? null }) }} />
          {subnetId && (
            <div className="ml-auto flex gap-1" role="group" aria-label="View">
              <Button variant={view === 'table' ? 'primary' : 'secondary'} aria-pressed={view === 'table'} onClick={() => set({ view: null })}><List size={14} aria-hidden /> Table</Button>
              <Button variant={view === 'grid' ? 'primary' : 'secondary'} aria-pressed={view === 'grid'} onClick={() => set({ view: 'grid' })}><Grid3x3 size={14} aria-hidden /> Grid</Button>
            </div>
          )}
        </div>
        {view === 'grid' && current && <SubnetGrid key={current.id} subnet={current} onOpen={(id) => set({ id })} />}
        {view === 'table' && (
          <>
            {isPending && <SkeletonRows rows={8} />}
            {error && <ErrorState error={error} onRetry={refetch} />}
            {data?.data.length === 0 && (filtered
              ? <EmptyState title="No addresses match" body={subnetId ? 'Only addresses of interest have records. Switch to the grid to see every address in the subnet.' : undefined} />
              : <EmptyState title="No IP records yet" body="Records appear as addresses are allocated, imported or reported by approved sources." />)}
            {data && data.data.length > 0 && (
              <>
                <div className="overflow-x-auto">
                  <table className="w-full border-collapse">
                    <thead><tr><Th>Address</Th><Th>Device</Th><Th>OS</Th><Th>MAC</Th><Th>Subnet</Th><Th>Owner</Th><Th>State</Th><Th>Last seen</Th></tr></thead>
                    <tbody>
                      {data.data.map((r) => (
                        <tr key={r.id} className="cursor-pointer hover:bg-surface-2" onClick={() => set({ id: r.id })}>
                          <Td><button type="button" className="cursor-pointer hover:text-accent" onClick={() => set({ id: r.id })}><Mono>{r.address}</Mono></button></Td>
                          <Td>
                            <span className="flex items-center gap-2">
                              <DeviceIcon type={r.deviceType} />
                              <span className="min-w-0">
                                <span className="block truncate text-ink">{r.deviceName || r.hostname || '—'}</span>
                                {r.deviceType && <span className="block text-[11.5px] text-ink-3">{deviceTypes[r.deviceType]?.label}{r.vendor && ` · ${r.vendor}`}</span>}
                              </span>
                            </span>
                          </Td>
                          <Td><OSBadge family={r.osFamily} version={r.operatingSystem} /></Td>
                          <Td>{r.mac ? <Mono>{r.mac}</Mono> : <span className="text-ink-3">—</span>}</Td>
                          <Td className="whitespace-nowrap text-ink-2">{r.subnetName} <span className="text-ink-3">· {r.routingDomainName}</span></Td>
                          <Td className="text-ink-2">{r.ownerEmail ?? '—'}</Td>
                          <Td><StateBadges allocation={r.allocationState} observation={r.observationState} /></Td>
                          <Td className="whitespace-nowrap text-ink-2">{r.lastSeen ? formatDateTime(r.lastSeen) : '—'}</Td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
              </>
            )}
          </>
        )}
      </Card>
      {openId && <RecordModal key={openId} id={openId} onClose={() => set({ id: null })} />}
      {scanning && current && <StartJobModal subnet={current} onClose={() => setScanning(false)} />}
      {creating && <NewRecord onClose={() => setCreating(false)} onCreated={(id) => set({ id })} />}
    </>
  )
}
