import { useQuery } from '@tanstack/react-query'
import { Grid3x3, List, Plus, Radar } from 'lucide-react'
import { useState } from 'react'
import { useSearchParams } from 'react-router'
import { Button, Card, ErrorState, PageHeader, SkeletonRows } from '../../components/ui'
import { RouteTabs } from '../../components/workflow'
import { get } from '../../lib/api'
import { useCan } from '../../lib/session'
import { useRouteTab } from '../../lib/tabs'
import type { Subnet } from '../../lib/types'
import { StartJobModal } from '../discovery/StartJobModal'
import { AddressTable } from './AddressTable'
import type { SetParams } from './AddressTable'
import { NewRecord, RecordModal } from './records'
import { SubnetCards } from './SubnetCards'
import { SubnetGrid } from './SubnetGrid'
import { SubnetSummary } from './SubnetSummary'

function SubnetView({ subnet, grid: wanted, params, set, onBack }: { subnet: Subnet; grid: boolean; params: URLSearchParams; set: SetParams; onBack: () => void }) {
  // The grid walks the whole subnet, so it needs the whole inventory.
  const seesAll = useCan('inventory:view')
  const grid = wanted && seesAll
  const toggle = seesAll && (
    <div className="ml-auto flex gap-1" role="group" aria-label="View">
      <Button variant={grid ? 'secondary' : 'primary'} aria-pressed={!grid} onClick={() => set({ view: null })}><List size={14} aria-hidden /> Table</Button>
      <Button variant={grid ? 'primary' : 'secondary'} aria-pressed={grid} onClick={() => set({ view: 'grid' })}><Grid3x3 size={14} aria-hidden /> Grid</Button>
    </div>
  )
  return (
    <>
      <SubnetSummary subnet={subnet} onBack={onBack} />
      <Card>
        {grid
          ? <><div className="flex border-b border-line p-2.5">{toggle}</div><SubnetGrid key={subnet.id} subnet={subnet} onOpen={(id) => set({ id })} /></>
          : <AddressTable params={params} set={set} subnetId={subnet.id} toolbarEnd={toggle} />}
      </Card>
    </>
  )
}

export function Explorer() {
  const canCreate = useCan('inventory:create')
  const canScan = useCan('discovery:run')
  const [scanning, setScanning] = useState(false)
  const [creating, setCreating] = useState(false)
  const [params, setParams] = useSearchParams()
  const [opened, setOpened] = useState<Subnet | null>(null)
  const tab = useRouteTab(['ip-addresses', 'all'])
  const subnetId = params.get('subnetId') ?? ''
  const openId = params.get('id')
  const set: SetParams = (updates) => setParams((prev) => {
    const next = new URLSearchParams(prev)
    for (const [k, v] of Object.entries(updates)) {
      if (v) next.set(k, v)
      else next.delete(k)
    }
    if (!('page' in updates) && !('id' in updates) && !('view' in updates)) next.delete('page')
    return next
  })
  const pinned = useQuery({ queryKey: ['subnet', subnetId], enabled: Boolean(subnetId) && opened?.id !== subnetId, queryFn: () => get<Subnet>(`/subnets/${subnetId}`) })
  const current = opened?.id === subnetId ? opened : pinned.data ?? null
  const leave = () => set({ subnetId: null, view: null, allocationState: null, observationState: null, q: null })
  return (
    <>
      <PageHeader title="IP Address Explorer" description="Find a subnet, then see every tracked address, the device behind it, and its history. Allocation and observation are always shown separately."
        actions={<>
          {canScan && current && <Button title="Discover this subnet under an approved policy" onClick={() => setScanning(true)}><Radar size={15} aria-hidden /> Scan subnet</Button>}
          {canCreate && <Button variant="primary" onClick={() => setCreating(true)}><Plus size={15} aria-hidden /> New record</Button>}
        </>} />
      <RouteTabs tabs={[{ key: 'ip-addresses', label: 'Subnets' }, { key: 'all', label: 'All addresses' }]} active={subnetId ? 'ip-addresses' : tab} base="/ip-addresses" />
      {subnetId && !current && <Card>{pinned.error ? <ErrorState error={pinned.error} onRetry={pinned.refetch} /> : <SkeletonRows rows={6} />}</Card>}
      {current && <SubnetView key={current.id} subnet={current} grid={params.get('view') === 'grid'} params={params} set={set} onBack={leave} />}
      {!subnetId && tab === 'ip-addresses' && <SubnetCards onOpen={(s) => { setOpened(s); set({ subnetId: s.id }) }} />}
      {!subnetId && tab === 'all' && <Card><AddressTable params={params} set={set} subnetId="" /></Card>}
      {openId && <RecordModal key={openId} id={openId} onClose={() => set({ id: null })} />}
      {scanning && current && <StartJobModal subnet={current} onClose={() => setScanning(false)} />}
      {creating && <NewRecord onClose={() => setCreating(false)} onCreated={(id) => set({ id })} />}
    </>
  )
}
