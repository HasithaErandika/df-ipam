import { useState } from 'react'
import { Card, DataTable, FilterBar, FilterSelect, PageHeader, PathTabs, SearchInput, Segmented } from '../../components/ui'
import { useFilters, usePagedList } from '../../lib/listing'
import { useSegments } from '../../lib/queries'
import { useCan } from '../../lib/session'
import { useRouteTab } from '../../lib/tabs'
import { options, sources } from '../../lib/labels'
import { platformOf } from '../../lib/platforms'
import type { Finding } from '../../lib/types'
import { ApproveDeviceModal, type DecisionTarget, useRejectDevice } from '../shared/DeviceDecision'
import { RegisterIPModal, type RegisterPrefill } from '../shared/RegisterIPModal'
import { findingColumns } from './findingColumns'
import { ScanStatusPanel, ScanSummary } from './ScanSummary'

const kinds = [
  { value: 'online', text: 'Online now' }, { value: 'offline', text: 'Not answering' },
  { value: 'unregistered', text: 'Not in the register' }, { value: 'registered', text: 'In the register' },
  { value: 'static', text: 'Static on a floor' }, { value: 'unknown', text: 'Unknown device' },
  { value: 'rejected', text: 'Rejected device' }, { value: 'private', text: 'Private MAC' },
  { value: 'dhcp', text: 'DHCP lease' }, { value: 'vpn', text: 'VPN client' }, { value: 'alerting', text: 'Has open alerts' },
]

const windows = [{ value: '1h', text: '1 h' }, { value: '24h', text: '24 h' }, { value: '7d', text: '7 days' }, { value: '30d', text: '30 days' }, { value: 'all', text: 'All' }]

function Findings() {
  const segments = useSegments()
  const canEdit = useCan('inventory:edit')
  const f = useFilters(['segmentId', 'kind', 'source', 'seen', 'q', 'sort'] as const)
  const seen = f.values.seen || '24h'
  const { query, rows, meta, paging } = usePagedList<Finding>('/discovery/findings', { ...f.values, seen }, { refetchInterval: 60_000 })
  const [prefill, setPrefill] = useState<RegisterPrefill | null>(null)
  const [approve, setApprove] = useState<DecisionTarget | null>(null)
  const reject = useRejectDevice()
  const columns = findingColumns({
    canEdit,
    onRegister: (r) => setPrefill({ segmentId: r.segmentId ?? undefined, ip: r.ip, mac: r.mac, name: r.hostname || r.deviceName, platform: platformOf(r.platform, r.vendor, r.hostname)?.platform }),
    onApprove: (r) => setApprove({ id: r.deviceId!, mac: r.mac!, name: r.deviceName, hostname: r.hostname, vendor: r.vendor, platform: r.platform, lastIp: r.ip, segmentId: r.segmentId, segmentName: r.segmentName, registered: Boolean(r.registerId) }),
    onReject: (r) => void reject({ id: r.deviceId!, mac: r.mac! }),
  })
  return (
    <Card>
      <FilterBar active={f.active} onClear={f.clear}
        end={<Segmented label="Seen within" value={seen} onChange={(v) => f.set('seen', v === '24h' ? '' : v)} options={windows} />}>
        <SearchInput value={f.values.q} onChange={(v) => f.set('q', v)} placeholder="Search IP, MAC, hostname, vendor" />
        <FilterSelect label="Segment" value={f.values.segmentId} onChange={(v) => f.set('segmentId', v)} options={(segments.data ?? []).map((s) => ({ value: s.id, text: s.name }))} />
        <FilterSelect label="Finding" value={f.values.kind} onChange={(v) => f.set('kind', v)} options={kinds} anyLabel="All findings" />
        <FilterSelect label="Source" value={f.values.source} onChange={(v) => f.set('source', v)} options={options(sources)} />
        <FilterSelect label="Sort" value={f.values.sort} onChange={(v) => f.set('sort', v)} anyLabel="Newest first" options={[
          { value: 'ip', text: 'By IP' }, { value: '-firstSeen', text: 'First seen, newest' }, { value: 'firstSeen', text: 'First seen, oldest' }, { value: 'hostname', text: 'By hostname' }, { value: 'segment', text: 'By segment' },
        ]} />
      </FilterBar>
      <DataTable columns={columns} rows={rows} query={query} rowKey={(r) => r.id} meta={meta} paging={paging} filtered={f}
        empty={{ title: 'Nothing seen in this window', body: 'Findings appear after a scan. Try a longer time window, or start a scan.' }} />
      <RegisterIPModal open={prefill !== null} prefill={prefill ?? undefined} onClose={() => setPrefill(null)} />
      <ApproveDeviceModal target={approve} onClose={() => setApprove(null)} />
    </Card>
  )
}

export function DiscoveryPage() {
  const tab = useRouteTab(['discovery', 'scans'])
  return (
    <>
      <PageHeader title="Discovery" description="Everything the scanner and the firewall have seen. Evidence only: the register changes only when a person decides." actions={<ScanSummary compact />} />
      <PathTabs base="/discovery" tabs={[{ key: 'discovery', label: 'Findings' }, { key: 'scans', label: 'Scan status' }]} />
      {tab === 'scans' ? <ScanStatusPanel /> : <Findings />}
    </>
  )
}
