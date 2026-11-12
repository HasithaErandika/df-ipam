import { Badge, Card, DataTable, DeviceBadge, FilterBar, GatewayBadge, none, PrivateMacBadge, RegisterBadge, SearchInput, Segmented, Stack, type Column } from '../../components/ui'
import { useFilters, usePagedList } from '../../lib/listing'
import { WithPlatform } from '../../components/PlatformIcon'
import { knownPlatform, likelyPlatform } from '../../lib/platforms'
import type { SegmentAddress } from '../../lib/types'
import type { RegisterPrefill } from '../shared/RegisterIPModal'
import { Presence } from './findingColumns'

const states = [
  { value: '', text: 'All' }, { value: 'unregistered', text: 'Not registered' }, { value: 'registered', text: 'Registered' },
  { value: 'static', text: 'Static' }, { value: 'unknown', text: 'Unknown devices' },
]

/** Registered and seen addresses of one segment, register and evidence side by side. */
export function SegmentAddresses({ segmentId, onRegister }: { segmentId: string; onRegister?: (p: RegisterPrefill) => void }) {
  const f = useFilters(['state', 'q'] as const)
  const { query, rows, meta, paging } = usePagedList<SegmentAddress>(`/segments/${segmentId}/addresses`, f.values)
  const recent = (iso: string | null) => Boolean(iso && Date.now() - new Date(iso).getTime() < 30 * 60_000)
  const columns: Column<SegmentAddress>[] = [
    { key: 'ip', header: 'Address', cell: (r) => <div className="flex items-center gap-2"><span className="font-mono text-[12.5px]">{r.ip}</span>{r.inPool && <Badge>DHCP pool</Badge>}</div> },
    { key: 'register', header: 'Register', cell: (r) => <Stack main={<RegisterBadge status={r.registerStatus} />} sub={[r.assignedName, r.owner].filter(Boolean).join(' · ') || undefined} /> },
    { key: 'device', header: 'Seen as', cell: (r) => (r.mac || r.hostname
      ? <WithPlatform platform={r.platform} vendor={r.vendor} hostname={r.hostname ?? r.deviceName} gateway={r.isGateway}><div className="flex items-center gap-2 truncate">{r.deviceName || r.hostname || (r.isGateway ? 'Router' : <span className="text-ink-3">No name</span>)}{r.isGateway && <GatewayBadge />}</div>
        <div className="flex items-center gap-2 font-mono text-[11.5px] text-ink-3">{r.mac ?? ''}{r.isPrivateMac && <PrivateMacBadge />}</div></WithPlatform>
      : none) },
    { key: 'approval', header: 'Approval', hideBelow: 'lg', cell: (r) => (r.mac && !r.isPrivateMac ? <DeviceBadge status={r.deviceStatus} /> : none) },
    { key: 'presence', header: 'Presence', cell: (r) => (r.lastSeen ? <Presence online={recent(r.lastSeen)} firstSeen={r.firstSeen} lastSeen={r.lastSeen} /> : <span className="text-[12.5px] text-ink-3">Never seen</span>) },
    { key: 'act', header: '', className: 'text-right', cell: (r) => (onRegister && !r.registerId
      ? <button type="button" className="cursor-pointer text-[12.5px] font-medium text-accent hover:underline" onClick={() => onRegister({ segmentId, ip: r.ip, mac: r.mac, name: r.hostname ?? r.deviceName, platform: knownPlatform(r.platform), platformGuess: likelyPlatform(r) })}>Add to register</button>
      : null) },
  ]
  return (
    <Card title="Addresses" subtitle="Every address that is registered or has been seen" className="mt-4">
      <FilterBar active={f.active} onClear={f.clear} end={<Segmented label="Show" value={f.values.state} onChange={(v) => f.set('state', v)} options={states} />}>
        <SearchInput value={f.values.q} onChange={(v) => f.set('q', v)} placeholder="Search IP, MAC, name" />
      </FilterBar>
      <DataTable columns={columns} rows={rows} query={query} rowKey={(r) => r.ip} meta={meta} paging={paging} filtered={f}
        empty={{ title: 'No addresses yet', body: 'Addresses appear here once they are registered or seen by a scan.' }} />
    </Card>
  )
}
