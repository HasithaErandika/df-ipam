import { useQuery } from '@tanstack/react-query'
import { CheckCircle2, Pencil, Plus, XCircle } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router'
import { Button, Card, DataTable, DeviceBadge, FilterBar, FilterSelect, none, PageHeader, PathTabs, SearchInput, Stack, RowMenu, type Column } from '../../components/ui'
import { list } from '../../lib/api'
import { deviceTypes, label, options } from '../../lib/labels'
import { useFilters, usePagedList } from '../../lib/listing'
import { inventoryKeys } from '../../lib/queries'
import { useCan } from '../../lib/session'
import { useRouteTab } from '../../lib/tabs'
import type { Device } from '../../lib/types'
import { WithPlatform } from '../../components/PlatformIcon'
import { Presence } from '../discovery/findingColumns'
import { ApproveDeviceModal, type DecisionTarget, useRejectDevice } from '../shared/DeviceDecision'
import { ExportMenu, ImportButton } from '../shared/FileTransfer'
import { DeviceFormModal } from './DeviceFormModal'

const tabs = ['devices', 'approved', 'rejected', 'all']

export function DevicesPage() {
  const tab = useRouteTab(tabs)
  const status = tab === 'devices' ? 'pending' : tab === 'all' ? '' : tab
  const canEdit = useCan('inventory:edit')
  const f = useFilters(['deviceType', 'q', 'sort'] as const)
  const { query, rows, meta, paging } = usePagedList<Device>('/devices', { ...f.values, status })
  const pending = useQuery({ queryKey: ['/devices', 'pending-count'], queryFn: () => list<Device>('/devices?status=pending&pageSize=1') })
  const [form, setForm] = useState<{ device?: Device } | null>(null)
  const [approve, setApprove] = useState<DecisionTarget | null>(null)
  const reject = useRejectDevice()

  const columns: Column<Device>[] = [
    { key: 'device', header: 'Device', cell: (d) => (
      <WithPlatform platform={d.platform} vendor={d.vendor} hostname={d.hostname ?? d.name} type={d.deviceType}>
        <div className="truncate text-ink">{d.name || d.hostname || <span className="text-ink-3">Unnamed</span>}</div>
        <div className="font-mono text-[11.5px] text-ink-3">{d.mac}</div>
        {d.vendor && <div className="truncate text-[11.5px] text-ink-3" title="DHCP vendor class">{d.vendor}</div>}
      </WithPlatform>
    ) },
    { key: 'type', header: 'Type', hideBelow: 'md', cell: (d) => label(deviceTypes, d.deviceType) },
    { key: 'owner', header: 'Owner', hideBelow: 'lg', cell: (d) => <Stack main={d.owner || none} sub={d.department || undefined} /> },
    { key: 'ip', header: 'Last address', cell: (d) => (d.lastIp
      ? <Stack mono main={d.lastIp} sub={d.segmentId ? <Link className="font-sans hover:text-accent hover:underline" to={`/segments/${d.segmentId}`}>{d.segmentName}</Link> : undefined} />
      : none) },
    { key: 'presence', header: 'Presence', cell: (d) => <Presence online={d.online} firstSeen={d.firstSeen} lastSeen={d.lastSeen} /> },
    { key: 'status', header: 'Status', cell: (d) => <DeviceBadge status={d.status} /> },
    { key: 'actions', header: '', className: 'text-right whitespace-nowrap', cell: (d) => (!canEdit ? null : d.status === 'pending' ? (
      <div className="flex justify-end gap-1">
        <Button onClick={() => setApprove({ ...d })}><CheckCircle2 size={14} aria-hidden />Approve</Button>
        <Button variant="ghost" className="text-danger" onClick={() => void reject(d)}>Reject</Button>
      </div>
    ) : (
      <RowMenu actions={[
        { label: 'Edit', icon: <Pencil size={14} aria-hidden />, onSelect: () => setForm({ device: d }) },
        { label: 'Approve', icon: <CheckCircle2 size={14} aria-hidden />, onSelect: () => setApprove({ ...d }), hidden: d.status === 'approved' },
        { label: 'Reject', icon: <XCircle size={14} aria-hidden />, onSelect: () => void reject(d), danger: true, hidden: d.status === 'rejected' },
      ]} />
    )) },
  ]

  return (
    <>
      <PageHeader title="Devices" description="Known MAC addresses. New ones found on the network wait in Pending until someone approves or rejects them."
        actions={<>
          <ExportMenu path="/devices/export" name="devices" />
          {canEdit && <ImportButton path="/devices/import" title="Import devices" required="mac" columns={['mac', 'name', 'owner', 'department', 'deviceType', 'notes']} refreshKeys={inventoryKeys} />}
          {canEdit && <Button variant="primary" onClick={() => setForm({})}><Plus size={14} aria-hidden />Add device</Button>}
        </>} />
      <PathTabs base="/devices" tabs={[{ key: 'devices', label: 'Pending', count: pending.data?.meta.total }, { key: 'approved', label: 'Approved' }, { key: 'rejected', label: 'Rejected' }, { key: 'all', label: 'All' }]} />
      <Card>
        <FilterBar active={f.active} onClear={f.clear}>
          <SearchInput value={f.values.q} onChange={(v) => f.set('q', v)} placeholder="Search MAC, name, owner, IP" />
          <FilterSelect label="Type" value={f.values.deviceType} onChange={(v) => f.set('deviceType', v)} options={options(deviceTypes)} />
          <FilterSelect label="Sort" value={f.values.sort} onChange={(v) => f.set('sort', v)} anyLabel="Most recent" options={[{ value: 'name', text: 'By name' }, { value: 'mac', text: 'By MAC' }, { value: 'lastSeen', text: 'Seen longest ago' }]} />
        </FilterBar>
        <DataTable columns={columns} rows={rows} query={query} rowKey={(d) => d.id} meta={meta} paging={paging} filtered={f}
          empty={tab === 'devices'
            ? { title: 'Nothing waiting for review', body: 'New MAC addresses seen on the network appear here. Randomised (private) MACs are left out.' }
            : { title: 'No devices here yet', body: 'Add devices by hand, or import your asset list from a CSV or Excel file.' }} />
      </Card>
      <DeviceFormModal open={form !== null} device={form?.device} onClose={() => setForm(null)} />
      <ApproveDeviceModal target={approve} onClose={() => setApprove(null)} />
    </>
  )
}
