import { AlertTriangle, CheckCircle2, PlusCircle, XCircle } from 'lucide-react'
import { Link } from 'react-router'
import { WithPlatform } from '../../components/PlatformIcon'
import { DeviceBadge, none, PrivateMacBadge, RegisterBadge, RowMenu, Stack, StateBadge, type Column } from '../../components/ui'
import { formatAgo, formatDateTime } from '../../lib/format'
import { label, sources } from '../../lib/labels'
import type { Finding } from '../../lib/types'

/** First seen, last seen, and how long it has been silent once it stops answering. */
export function Presence({ online, firstSeen, lastSeen }: { online: boolean; firstSeen: string | null; lastSeen: string | null }) {
  if (!lastSeen) return none
  return (
    <div className="min-w-0">
      {online
        ? <StateBadge tone="ok" title={`Last answered ${formatDateTime(lastSeen)}`}>Online</StateBadge>
        : <StateBadge tone="neutral" title={`Last answered ${formatDateTime(lastSeen)}`}>Unavailable since {formatAgo(lastSeen)}</StateBadge>}
      {firstSeen && <div className="mt-0.5 text-[11.5px] text-ink-3" title={formatDateTime(firstSeen)}>First discovered {formatAgo(firstSeen)}</div>}
    </div>
  )
}

export function findingColumns({ canEdit, onRegister, onApprove, onReject }: {
  canEdit: boolean
  onRegister: (r: Finding) => void
  onApprove: (r: Finding) => void
  onReject: (r: Finding) => void
}): Column<Finding>[] {
  return [
    { key: 'ip', header: 'Address', cell: (r) => <Stack mono main={r.ip} sub={r.segmentId ? <Link className="font-sans hover:text-accent hover:underline" to={`/segments/${r.segmentId}`}>{r.segmentName}</Link> : 'No segment'} /> },
    { key: 'device', header: 'Device', cell: (r) => (
      <WithPlatform platform={r.platform} vendor={r.vendor} hostname={r.hostname || r.deviceName}>
        <div className="truncate text-ink">{r.deviceName || r.hostname || <span className="text-ink-3">No name found</span>}</div>
        <div className="flex items-center gap-2 font-mono text-[11.5px] text-ink-3">{r.mac ?? 'MAC not visible'}{r.isPrivateMac && <PrivateMacBadge />}</div>
        {r.vendor && <div className="truncate text-[11.5px] text-ink-3" title="DHCP vendor class">{r.vendor}</div>}
      </WithPlatform>
    ) },
    { key: 'register', header: 'Register', cell: (r) => <Stack main={<RegisterBadge status={r.registerStatus} />} sub={r.assignedName ?? undefined} /> },
    { key: 'approval', header: 'Approval', hideBelow: 'lg', cell: (r) => (r.mac && !r.isPrivateMac ? <DeviceBadge status={r.deviceStatus} /> : none) },
    { key: 'presence', header: 'Presence', cell: (r) => <Presence online={r.online} firstSeen={r.firstSeen} lastSeen={r.lastSeen} /> },
    { key: 'source', header: 'Source', hideBelow: 'xl', cell: (r) => <Stack main={label(sources, r.source)} sub={r.segmentKind === 'dhcp' ? (r.isStatic ? 'Static (outside pool)' : 'DHCP pool') : undefined} /> },
    { key: 'alerts', header: '', className: 'w-8', cell: (r) => (r.openAlerts > 0
      ? <Link to={`/alerts?q=${encodeURIComponent(r.ip)}`} title={`${r.openAlerts} open alert${r.openAlerts > 1 ? 's' : ''}`} className="text-warn"><AlertTriangle size={15} aria-label="Open alerts" /></Link>
      : null) },
    { key: 'actions', header: '', className: 'w-10 text-right', cell: (r) => (
      <RowMenu actions={[
        { label: 'Add to register', icon: <PlusCircle size={14} aria-hidden />, onSelect: () => onRegister(r), hidden: !canEdit || Boolean(r.registerId) || !r.segmentId },
        { label: 'Approve device', icon: <CheckCircle2 size={14} aria-hidden />, onSelect: () => onApprove(r), hidden: !canEdit || !r.deviceId || r.deviceStatus === 'approved' },
        { label: 'Reject device', icon: <XCircle size={14} aria-hidden />, onSelect: () => onReject(r), danger: true, hidden: !canEdit || !r.deviceId || r.deviceStatus !== 'pending' },
      ]} />
    ) },
  ]
}
