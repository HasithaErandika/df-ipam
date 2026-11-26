import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useDialogParam } from '../../lib/dialogParam'
import { Card, DataTable, FilterBar, FilterSelect, PageHeader, PathTabs, SearchInput, Segmented } from '../../components/ui'
import { list } from '../../lib/api'
import { useFilters, usePagedList } from '../../lib/listing'
import { useSegments } from '../../lib/queries'
import { useCan } from '../../lib/session'
import { useRouteTab } from '../../lib/tabs'
import { options, sources } from '../../lib/labels'
import type { Finding } from '../../lib/types'
import { ApproveDeviceModal, type DecisionTarget, useRejectDevice } from '../shared/DeviceDecision'
import { RegisterIPModal, type RegisterPrefill } from '../shared/RegisterIPModal'
import { approveTarget, registerPrefill } from './prefill'
import { DiscoverySummary } from './DiscoverySummary'
import { FindingDetails } from './FindingDetails'
import { attentionRow, findingColumns } from './findingColumns'
import { ScanStatusPanel, ScanSummary } from './ScanSummary'
import { SegmentsOverview } from './SegmentsOverview'

const kinds = [
  { value: 'needs', text: 'Needs action' }, { value: 'new', text: 'New in the last hour' },
  { value: 'known', text: 'Known (registered and approved)' }, { value: 'offline', text: 'Left (not answering)' },
  { value: 'unregistered', text: 'Not in the register' }, { value: 'registered', text: 'In the register' },
  { value: 'static', text: 'Static in a DHCP network' }, { value: 'unknown', text: 'Unknown device' },
  { value: 'rejected', text: 'Rejected device' }, { value: 'private', text: 'Private MAC' },
  { value: 'gateway', text: 'Gateways' }, { value: 'dhcp', text: 'DHCP lease' }, { value: 'vpn', text: 'VPN client' }, { value: 'alerting', text: 'Has open alerts' },
]

const windows = [{ value: 'now', text: 'Connected now' }, { value: '24h', text: '24 h' }, { value: '7d', text: '7 days' }, { value: '30d', text: '30 days' }, { value: 'all', text: 'All' }]

function Findings() {
  const segments = useSegments()
  const canEdit = useCan('inventory:edit')
  const f = useFilters(['segmentId', 'kind', 'source', 'seen', 'q', 'sort'] as const)
  const seen = f.values.seen || 'now'
  const [prefill, setPrefill] = useState<RegisterPrefill | null>(null)
  const [approve, setApprove] = useState<DecisionTarget | null>(null)
  const reject = useRejectDevice()
  const [openId, setOpen] = useDialogParam('finding')
  // The list stops refreshing while a dialog is open, so rows don't move under it.
  const busy = Boolean(openId || prefill || approve)
  const { query, rows, meta, paging } = usePagedList<Finding>('/discovery/findings', { ...f.values, seen }, { refetchInterval: busy ? undefined : 30_000 })
  // The open finding is fetched on its own: a shared link works whatever page or
  // filter is showing, and the dialog stays open if the device stops answering.
  const opened = useQuery({
    queryKey: ['/discovery/findings', 'one', openId],
    queryFn: () => list<Finding>(`/discovery/findings?seen=all&id=${encodeURIComponent(openId!)}`),
    enabled: Boolean(openId),
    placeholderData: () => { const row = rows.find((r) => r.id === openId); return row ? { data: [row], meta: { page: 1, pageSize: 1, total: 1, totalPages: 1, hasNext: false, hasPrev: false } } : undefined },
  })
  const setKind = (v: string) => { f.set('kind', v); if (v === 'offline' && seen === 'now') f.set('seen', '24h') }
  const handlers = {
    canEdit,
    onOpen: (r: Finding) => setOpen(r.id),
    onRegister: (r: Finding) => setPrefill(registerPrefill(r)),
    onApprove: (r: Finding) => setApprove(approveTarget(r)),
    onReject: (r: Finding) => void reject({ id: r.deviceId!, mac: r.mac! }),
  }
  const columns = findingColumns(handlers)
  return (
    <Card>
      <FilterBar active={f.active} onClear={f.clear}
        end={<Segmented label="Seen within" value={seen} onChange={(v) => f.set('seen', v === 'now' ? '' : v)} options={windows} />}>
        <SearchInput value={f.values.q} onChange={(v) => f.set('q', v)} placeholder="Search IP, MAC, hostname, vendor" />
        <FilterSelect label="Segment" value={f.values.segmentId} onChange={(v) => f.set('segmentId', v)} options={(segments.data ?? []).map((s) => ({ value: s.id, text: s.name }))} />
        <FilterSelect label="Finding" value={f.values.kind} onChange={setKind} options={kinds} anyLabel="All findings" />
        <FilterSelect label="Source" value={f.values.source} onChange={(v) => f.set('source', v)} options={options(sources)} />
        <FilterSelect label="Sort" value={f.values.sort} onChange={(v) => f.set('sort', v)} anyLabel="Last seen, newest" options={[
          { value: 'ip', text: 'By IP' }, { value: '-firstSeen', text: 'First seen, newest' }, { value: 'firstSeen', text: 'First seen, oldest' }, { value: 'hostname', text: 'By hostname' }, { value: 'segment', text: 'By segment' },
        ]} />
      </FilterBar>
      <DiscoverySummary segmentId={f.values.segmentId} seen={seen} kind={f.values.kind} onKind={setKind} />
      <DataTable columns={columns} rows={rows} query={query} rowKey={(r) => r.id} meta={meta} paging={paging} filtered={f} rowClassName={attentionRow}
        empty={seen === 'now'
          ? { title: 'Nothing connected right now', body: 'Devices appear here while they answer scans, and drop off when they leave. Choose 24 h or longer to see recent history, or start a scan.' }
          : { title: 'Nothing seen in this window', body: 'Try a longer time window, or start a scan.' }} />
      <RegisterIPModal open={prefill !== null} prefill={prefill ?? undefined} onClose={() => setPrefill(null)} />
      <ApproveDeviceModal target={approve} onClose={() => setApprove(null)} />
      <FindingDetails row={openId ? opened.data?.data[0] ?? null : null} {...handlers} onClose={() => setOpen(null)} />
    </Card>
  )
}

export function DiscoveryPage() {
  const tab = useRouteTab(['discovery', 'segments', 'scans'])
  return (
    <>
      <PageHeader title="Discovery" description="Everything the scanner and the firewall have seen. Evidence only: the register changes only when a person decides." actions={<ScanSummary compact />} />
      <PathTabs base="/discovery" tabs={[{ key: 'discovery', label: 'Findings' }, { key: 'segments', label: 'Segments' }, { key: 'scans', label: 'Scan status' }]} />
      {tab === 'scans' ? <ScanStatusPanel /> : tab === 'segments' ? <SegmentsOverview /> : <Findings />}
    </>
  )
}
