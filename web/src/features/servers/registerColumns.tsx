import { Pencil, Trash2 } from 'lucide-react'
import { Link } from 'react-router'
import { ServiceIcons, WithPlatform } from '../../components/PlatformIcon'
import { Badge, EnvironmentBadge, HealthBadge, none, RegisterBadge, RowMenu, SeenBadge, Stack, type Column } from '../../components/ui'
import { ipTypes, label } from '../../lib/labels'
import type { RegisterEntry } from '../../lib/types'

interface Actions { canEdit: boolean; onEdit: (r: RegisterEntry) => void; onRelease: (r: RegisterEntry) => void }

const menu = ({ canEdit, onEdit, onRelease }: Actions): Column<RegisterEntry> => ({
  key: 'actions', header: '', className: 'w-10 text-right', cell: (r) => (canEdit ? (
    <RowMenu actions={[
      { label: 'Edit', icon: <Pencil size={14} aria-hidden />, onSelect: () => onEdit(r) },
      { label: 'Release address', icon: <Trash2 size={14} aria-hidden />, onSelect: () => onRelease(r), danger: true },
    ]} />
  ) : null),
})

const segmentLink = (r: RegisterEntry) => <Link className="hover:text-accent hover:underline" to={`/segments/${r.segmentId}`}>{r.segmentName}</Link>

/** Monitored servers: health first, because that is what people look for. */
export function serverColumns(a: Actions): Column<RegisterEntry>[] {
  return [
    { key: 'health', header: 'Status', cell: (r) => (r.monitor ? <HealthBadge health={r.health} since={r.healthChangedAt} /> : <span className="text-[12.5px] text-ink-3">Not monitored</span>) },
    { key: 'name', header: 'Server', cell: (r) => (
      <WithPlatform platform={r.platform} type="server">
        <div className="truncate text-ink">{r.assignedName || none}</div>
        <div className="flex items-center gap-2"><span className="font-mono text-[12px] text-ink-3">{r.ip}</span><ServiceIcons list={r.services} /></div>
      </WithPlatform>
    ) },
    { key: 'env', header: 'Environment', cell: (r) => <EnvironmentBadge name={r.environmentName} colour={r.environmentColour} /> },
    { key: 'critical', header: 'Critical', hideBelow: 'md', cell: (r) => (r.critical ? <Badge tone="danger">Critical</Badge> : none) },
    { key: 'check', header: 'Check', hideBelow: 'lg', cell: (r) => (r.monitor ? <span className="text-[12.5px] text-ink-2">{r.checkPort ? `Ping, then TCP ${r.checkPort}` : 'Ping'}</span> : none) },
    { key: 'segment', header: 'Segment', hideBelow: 'lg', cell: segmentLink },
    { key: 'seen', header: 'On the network', hideBelow: 'xl', cell: (r) => <SeenBadge lastSeen={r.lastSeen} /> },
    menu(a),
  ]
}

/** The whole register: what each address is for, with evidence beside it. */
export function registerColumns(a: Actions): Column<RegisterEntry>[] {
  return [
    { key: 'ip', header: 'Address', cell: (r) => <Stack mono main={r.ip} sub={<span className="font-sans">{segmentLink(r)}</span>} /> },
    { key: 'status', header: 'Register', cell: (r) => <RegisterBadge status={r.status} /> },
    { key: 'name', header: 'Assigned to', cell: (r) => <WithPlatform platform={r.platform} type={r.ipType}><Stack main={r.assignedName || none} sub={label(ipTypes, r.ipType)} /></WithPlatform> },
    { key: 'owner', header: 'Owner', hideBelow: 'lg', cell: (r) => <Stack main={r.owner || none} sub={r.department || undefined} /> },
    { key: 'env', header: 'Environment', hideBelow: 'md', cell: (r) => <EnvironmentBadge name={r.environmentName} colour={r.environmentColour} /> },
    { key: 'mac', header: 'Expected MAC', hideBelow: 'xl', cell: (r) => (r.expectedMac ? <span className="font-mono text-[12px]">{r.expectedMac}</span> : none) },
    { key: 'seen', header: 'On the network', hideBelow: 'md', cell: (r) => <SeenBadge lastSeen={r.lastSeen} /> },
    menu(a),
  ]
}
