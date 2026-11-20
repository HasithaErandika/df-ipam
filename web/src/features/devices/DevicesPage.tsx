import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, CheckCircle2, Info, Pencil, Plus, XCircle } from 'lucide-react'
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
import { deviceLine, iconEvidence, networkCard, serviceName } from '../discovery/deviceInfo'
import { Presence } from '../discovery/findingColumns'
import { useDialogParam } from '../../lib/dialogParam'
import { DeviceDetails } from './DeviceDetails'
import { ApproveDeviceModal, type DecisionTarget, useRejectDevice } from '../shared/DeviceDecision'
import { ExportMenu, ImportButton } from '../shared/FileTransfer'
import { DeviceFormModal } from './DeviceFormModal'
import { OwnerName } from '../shared/OwnerName'

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
  const [openId, setOpen] = useDialogParam('device')

  const actionsWidth = !canEdit ? 'w-24' : rows.some((d) => d.status === 'pending') ? 'w-48' : 'w-12'
  const columns: Column<Device>[] = [
    { key: 'device', header: 'Device', cell: (d) => (
      <WithPlatform evidence={iconEvidence(d)} type={d.deviceType}>
        <button type="button" onClick={() => setOpen(d.id)} title="Show details" className="block max-w-full cursor-pointer truncate text-left font-medium text-ink hover:text-accent hover:underline">
          <OwnerName owner={d.owner} name={d.name || d.hostname} fallback={<span className="font-normal text-ink-3">Unnamed</span>} />
        </button>
        {deviceLine(d) && <div className="truncate text-[12px] text-ink-2">{deviceLine(d)}</div>}
        <div className="truncate text-[11.5px] text-ink-3"><span className="font-mono">{d.mac}</span>{networkCard(d) && ` · card: ${networkCard(d)}`}{d.isPrivateMac && ' · private MAC'}</div>
        {d.openPorts.length > 0 && <div className="truncate text-[11.5px] text-ink-3" title={d.openPorts.join(', ')}>Open: {d.openPorts.map(serviceName).join(', ')}</div>}
      </WithPlatform>
    ) },
    { key: 'type', header: 'Type', width: 'w-[11%]', hideBelow: 'md', cell: (d) => label(deviceTypes, d.deviceType) },
    { key: 'owner', header: 'Owner', width: 'w-[13%]', hideBelow: 'lg', cell: (d) => <Stack main={d.owner || none} sub={d.department || undefined} /> },
    { key: 'ip', header: 'Last seen address', width: 'w-[19%]', cell: (d) => (d.lastIp
      ? <Stack mono main={d.lastIp} sub={<span className="block font-sans whitespace-normal">
          {d.segmentId ? <Link className="hover:text-accent hover:underline" to={`/segments/${d.segmentId}`}>{d.segmentName}</Link> : null}
          <span className="block text-ink-3">Observed address; DHCP may assign a different one.</span>
          {d.registeredIp ? <span className="block text-ok">Register entry at {d.registeredIp}</span> : null}
          {d.addresses > 1 ? <span className="block">{d.addresses} addresses seen</span> : null}
        </span>} />
      : none) },
    { key: 'presence', header: 'Presence', width: 'w-[13%]', cell: (d) => <Presence online={d.online} firstSeen={d.firstSeen} lastSeen={d.lastSeen} showFirstSeen={false} /> },
    { key: 'status', header: 'Status', width: 'w-[10%]', cell: (d) => <DeviceBadge status={d.status} /> },
    { key: 'alerts', header: '', width: 'w-10', cell: (d) => (d.openAlerts > 0
      ? <Link to={`/alerts?q=${encodeURIComponent(d.mac)}`} title={`${d.openAlerts} open alert${d.openAlerts > 1 ? 's' : ''}`} className="text-warn"><AlertTriangle size={15} aria-label="Open alerts" /></Link>
      : null) },
    { key: 'actions', header: '', width: actionsWidth, className: 'text-right whitespace-nowrap', cell: (d) => (!canEdit ? <Button variant="ghost" onClick={() => setOpen(d.id)}>Details</Button> : d.status === 'pending' ? (
      <div className="flex justify-end gap-1">
        <Button onClick={() => setApprove({ ...d })}><CheckCircle2 size={14} aria-hidden />Approve</Button>
        <Button variant="ghost" className="text-danger" onClick={() => void reject(d)}>Reject</Button>
      </div>
    ) : (
      <RowMenu actions={[
        { label: 'Details', icon: <Info size={14} aria-hidden />, onSelect: () => setOpen(d.id) },
        { label: 'Edit', icon: <Pencil size={14} aria-hidden />, onSelect: () => setForm({ device: d }) },
        { label: 'Approve', icon: <CheckCircle2 size={14} aria-hidden />, onSelect: () => setApprove({ ...d }), hidden: d.status === 'approved' },
        { label: 'Reject', icon: <XCircle size={14} aria-hidden />, onSelect: () => void reject(d), danger: true, hidden: d.status === 'rejected' },
      ]} />
    )) },
  ]

  return (
    <>
      <PageHeader title="Devices" description="Known MAC addresses. The last-seen address is scanner evidence and may change with DHCP; the IP register records intentional assignments. New devices wait in Pending for review."
        actions={<>
          <ExportMenu path="/devices/export" name="devices" />
          {canEdit && <ImportButton path="/devices/import" title="Import devices" keyLabel="MAC" required="mac" columns={['mac', 'name', 'owner', 'department', 'deviceType', 'notes']} refreshKeys={inventoryKeys} />}
          {canEdit && <Button variant="primary" onClick={() => setForm({})}><Plus size={14} aria-hidden />Add device</Button>}
        </>} />
      <PathTabs base="/devices" tabs={[{ key: 'devices', label: 'Pending', count: pending.data?.meta.total }, { key: 'approved', label: 'Approved' }, { key: 'rejected', label: 'Rejected' }, { key: 'all', label: 'All' }]} />
      <Card>
        <FilterBar active={f.active} onClear={f.clear}>
          <SearchInput value={f.values.q} onChange={(v) => f.set('q', v)} placeholder="Search MAC, name, owner, IP" />
          <FilterSelect label="Type" value={f.values.deviceType} onChange={(v) => f.set('deviceType', v)} options={options(deviceTypes)} />
          <FilterSelect label="Sort" value={f.values.sort} onChange={(v) => f.set('sort', v)} anyLabel="Most recent" options={[{ value: 'name', text: 'By name' }, { value: 'mac', text: 'By MAC' }, { value: 'lastSeen', text: 'Seen longest ago' }]} />
        </FilterBar>
        <DataTable fixed columns={columns} rows={rows} query={query} rowKey={(d) => d.id} meta={meta} paging={paging} filtered={f}
          empty={tab === 'devices'
            ? { title: 'Nothing waiting for review', body: 'New MAC addresses seen on the network appear here. Randomised (private) MACs are left out.' }
            : { title: 'No devices here yet', body: 'Add devices by hand, or import your asset list from a CSV or Excel file.' }} />
      </Card>
      <DeviceFormModal open={form !== null} device={form?.device} onClose={() => setForm(null)} />
      <ApproveDeviceModal target={approve} onClose={() => setApprove(null)} />
      <DeviceDetails device={rows.find((d) => d.id === openId) ?? null} canEdit={canEdit} onClose={() => setOpen(null)}
        onApprove={(d) => setApprove({ ...d })} onReject={(d) => void reject(d)} onEdit={(d) => setForm({ device: d })} />
    </>
  )
}
