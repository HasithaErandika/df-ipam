import { AlertTriangle, CheckCircle2, Info, PlusCircle, ShieldCheck, XCircle } from 'lucide-react'
import { Link } from 'react-router'
import { WithPlatform } from '../../components/PlatformIcon'
import { Button, DeviceBadge, GatewayBadge, none, PrivateMacBadge, RegisterBadge, RowMenu, Stack, StateBadge, type Column } from '../../components/ui'
import { formatAgo, formatDateTime } from '../../lib/format'
import { label, sources } from '../../lib/labels'
import type { Finding } from '../../lib/types'
import { deviceLine, networkCard, serviceName } from './deviceInfo'

/** First discovered within the last hour: a device that just joined. */
export const isNew = (firstSeen: string | null) => Boolean(firstSeen) && Date.now() - new Date(firstSeen!).getTime() < 3_600_000

/** Registered and approved, here or at its own address on another network: nothing to act on. */
export const isKnown = (r: Finding) => r.known

/** An approved device registered at another address, seen here (a laptop on another Wi-Fi). */
export const isRoaming = (r: Finding) => !r.registerId && Boolean(r.homeIp)

/** "Registered elsewhere", with where, for a roaming device. */
export function RegisterCell({ r }: { r: Finding }) {
  if (isRoaming(r)) {
    return <Stack main={<StateBadge tone="accent" title={`Registered at ${r.homeIp} on ${r.homeSegment}`}>Registered elsewhere</StateBadge>}
      sub={<span>at <span className="font-mono">{r.homeIp}</span> · {r.homeSegment}</span>} />
  }
  return <Stack main={<RegisterBadge status={r.registerStatus} />} sub={r.owner ? `Owner: ${r.owner}` : (r.assignedName ?? undefined)} />
}

/** First seen, last seen, and how long it has been silent once it stops answering. */
export function Presence({ online, firstSeen, lastSeen }: { online: boolean; firstSeen: string | null; lastSeen: string | null }) {
  if (!lastSeen) return none
  return (
    <div className="min-w-0">
      {online
        ? <StateBadge tone="ok" title={`Last answered ${formatDateTime(lastSeen)}`}>Online</StateBadge>
        : <StateBadge tone="neutral" title={`Last answered ${formatDateTime(lastSeen)}`}>Unavailable since {formatAgo(lastSeen)}</StateBadge>}
      {firstSeen && <div className="mt-0.5 flex items-center gap-1.5 text-[11.5px] text-ink-3" title={formatDateTime(firstSeen)}>First discovered {formatAgo(firstSeen)}{isNew(firstSeen) && <StateBadge tone="accent">New</StateBadge>}</div>}
    </div>
  )
}

/** The one decision this row most needs, shown as a button instead of hidden in the menu. */
function mainAction(r: Finding, canEdit: boolean): 'approve' | 'register' | null {
  if (!canEdit) return null
  if (r.deviceId && r.deviceStatus === 'pending') return 'approve'
  if (!r.registerId && !isRoaming(r) && r.segmentId && r.deviceStatus !== 'rejected' && !r.isGateway) return 'register'
  return null
}

export function findingColumns({ canEdit, onOpen, onRegister, onApprove, onReject }: {
  canEdit: boolean
  onOpen: (r: Finding) => void
  onRegister: (r: Finding) => void
  onApprove: (r: Finding) => void
  onReject: (r: Finding) => void
}): Column<Finding>[] {
  return [
    { key: 'ip', header: 'Address', cell: (r) => <Stack mono main={<button type="button" className="cursor-pointer hover:text-accent hover:underline" onClick={() => onOpen(r)} title="Show details">{r.ip}</button>} sub={r.segmentId ? <Link className="font-sans hover:text-accent hover:underline" to={`/segments/${r.segmentId}`}>{r.segmentName}</Link> : 'No segment'} /> },
    { key: 'device', header: 'Device', cell: (r) => (
      <WithPlatform platform={r.platform} vendor={deviceLine(r)} hostname={r.hostname || r.deviceName} gateway={r.isGateway}>
        <div className="flex items-center gap-2 truncate text-ink">
          <span className={r.deviceStatus === 'approved' && r.deviceName ? 'truncate font-medium' : 'truncate'}>{r.deviceName || r.assignedName || r.hostname || (r.isGateway ? 'Router' : <span className="text-ink-3">No name found</span>)}</span>
          {r.isGateway && <GatewayBadge />}
        </div>
        {(r.deviceName || r.assignedName) && r.hostname && r.hostname !== (r.deviceName || r.assignedName) && <div className="truncate text-[11.5px] text-ink-3" title="Hostname the device reported">{r.hostname}</div>}
        {deviceLine(r) && <div className="truncate text-[12px] text-ink-2" title="What the device says about itself">{deviceLine(r)}</div>}
        <div className="flex items-center gap-2 truncate text-[11.5px] text-ink-3">
          <span className="font-mono">{r.mac ?? 'MAC not visible'}</span>{r.isPrivateMac && <PrivateMacBadge />}
          {networkCard(r) && <span className="truncate" title="Maker of the network card, from the MAC. Often not the device's maker.">· card: {networkCard(r)}</span>}
        </div>
        {r.openPorts.length > 0 && <div className="truncate text-[11.5px] text-ink-3" title={r.openPorts.map((p) => `${p} ${serviceName(p)}`).join(', ')}>Open: {r.openPorts.map(serviceName).join(', ')}</div>}
      </WithPlatform>
    ) },
    { key: 'register', header: 'Register', cell: (r) => <RegisterCell r={r} /> },
    { key: 'approval', header: 'Approval', hideBelow: 'lg', cell: (r) => (r.deviceId || (r.mac && !r.isPrivateMac) ? <DeviceBadge status={r.deviceStatus} /> : none) },
    { key: 'presence', header: 'Presence', cell: (r) => <Presence online={r.online} firstSeen={r.firstSeen} lastSeen={r.lastSeen} /> },
    { key: 'source', header: 'Source', hideBelow: 'xl', cell: (r) => <Stack main={label(sources, r.source)} sub={r.segmentKind === 'dhcp' ? (r.isStatic ? 'Static (outside pool)' : 'DHCP pool') : undefined} /> },
    { key: 'alerts', header: '', className: 'w-8', cell: (r) => (r.openAlerts > 0
      ? <Link to={`/alerts?q=${encodeURIComponent(r.ip)}`} title={`${r.openAlerts} open alert${r.openAlerts > 1 ? 's' : ''}`} className="text-warn"><AlertTriangle size={15} aria-label="Open alerts" /></Link>
      : isKnown(r) ? <span title="Registered and approved: no warnings" className="text-ok"><ShieldCheck size={15} aria-label="Known device, no warnings" /></span> : null) },
    { key: 'actions', header: '', className: 'w-px whitespace-nowrap text-right', cell: (r) => (
      <div className="flex items-center justify-end gap-1">
      {mainAction(r, canEdit) === 'approve' && <Button className="h-7 px-2 text-[12px]" onClick={() => onApprove(r)}><CheckCircle2 size={13} aria-hidden />Approve</Button>}
      {mainAction(r, canEdit) === 'register' && <Button className="h-7 px-2 text-[12px]" onClick={() => onRegister(r)}><PlusCircle size={13} aria-hidden />Register</Button>}
      <RowMenu actions={[
        { label: 'Details', icon: <Info size={14} aria-hidden />, onSelect: () => onOpen(r) },
        { label: 'Add to register', icon: <PlusCircle size={14} aria-hidden />, onSelect: () => onRegister(r), hidden: !canEdit || Boolean(r.registerId) || !r.segmentId },
        { label: 'Approve device', icon: <CheckCircle2 size={14} aria-hidden />, onSelect: () => onApprove(r), hidden: !canEdit || !r.deviceId || r.deviceStatus === 'approved' },
        { label: 'Reject device', icon: <XCircle size={14} aria-hidden />, onSelect: () => onReject(r), danger: true, hidden: !canEdit || !r.deviceId || r.deviceStatus !== 'pending' },
      ]} />
      </div>
    ) },
  ]
}
