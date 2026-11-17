import { AlertTriangle, Info, Pencil, Trash2 } from 'lucide-react'
import { Link } from 'react-router'
import { ServiceIcons, WithPlatform } from '../../components/PlatformIcon'
import { Badge, EnvironmentBadge, HealthBadge, none, RegisterBadge, RowMenu, SeenBadge, Stack, type Column } from '../../components/ui'
import { ipTypes, label } from '../../lib/labels'
import type { RegisterEntry } from '../../lib/types'
import { deviceFacts, iconEvidence, serviceName } from '../discovery/deviceInfo'

interface Actions { canEdit: boolean; onOpen: (r: RegisterEntry) => void; onEdit: (r: RegisterEntry) => void; onRelease: (r: RegisterEntry) => void }

const menu = ({ canEdit, onOpen, onEdit, onRelease }: Actions): Column<RegisterEntry> => ({
  key: 'actions', header: '', className: 'w-10 text-right', cell: (r) => (
    <RowMenu actions={[
      { label: 'Details', icon: <Info size={14} aria-hidden />, onSelect: () => onOpen(r) },
      { label: 'Edit', icon: <Pencil size={14} aria-hidden />, onSelect: () => onEdit(r), hidden: !canEdit },
      { label: 'Release address', icon: <Trash2 size={14} aria-hidden />, onSelect: () => onRelease(r), danger: true, hidden: !canEdit },
    ]} />
  ),
})

const opener = (onOpen: Actions['onOpen'], r: RegisterEntry, text: React.ReactNode, mono = false) => (
  <button type="button" onClick={() => onOpen(r)} title="Show details"
    className={`block max-w-full cursor-pointer truncate text-left text-ink hover:text-accent hover:underline ${mono ? 'font-mono text-[12.5px]' : 'font-medium'}`}>{text}</button>
)

/** What the scanner saw at the address: name, likely system and open services. */
const seenAs: Column<RegisterEntry> = {
  key: 'seenAs', header: 'Seen as', hideBelow: 'xl', cell: (r) => {
    if (!r.lastSeen) return none
    const f = deviceFacts(r)
    const what = [f.maker, f.model, f.kind].filter(Boolean).join(' · ') || f.system.replace(/ \(.*\)$/, '')
    return (
      <div className="min-w-0 max-w-[16rem] text-[12px]">
        <div className="truncate text-ink-2">{r.hostname || what || 'Answering'}</div>
        {r.hostname && what && <div className="truncate text-ink-3">{what}</div>}
        {r.openPorts.length > 0 && <div className="truncate text-ink-3" title={r.openPorts.join(', ')}>Open: {r.openPorts.map(serviceName).join(', ')}</div>}
      </div>
    )
  },
}

const alerts: Column<RegisterEntry> = {
  key: 'alerts', header: '', className: 'w-8', cell: (r) => (r.openAlerts > 0
    ? <Link to={`/alerts?q=${encodeURIComponent(r.ip)}`} title={`${r.openAlerts} open alert${r.openAlerts > 1 ? 's' : ''}`} className="text-warn"><AlertTriangle size={15} aria-label="Open alerts" /></Link>
    : null),
}

const wrongMac = (r: RegisterEntry) => Boolean(r.expectedMac && r.seenMac && r.expectedMac.toLowerCase() !== r.seenMac.toLowerCase())

const segmentLink = (r: RegisterEntry) => <Link className="hover:text-accent hover:underline" to={`/segments/${r.segmentId}`}>{r.segmentName}</Link>

/** Monitored servers: health first, because that is what people look for. */
export function serverColumns(a: Actions): Column<RegisterEntry>[] {
  return [
    { key: 'health', header: 'Status', cell: (r) => (r.monitor
      ? <span className="inline-flex items-center gap-2"><HealthBadge health={r.health} since={r.healthChangedAt} />{r.health === 'up' && r.responseMs !== null && <span className="text-[12px] text-ink-3 tabular-nums">{r.responseMs} ms</span>}</span>
      : <span className="text-[12.5px] text-ink-3">Not monitored</span>) },
    { key: 'name', header: 'Server', cell: (r) => (
      <WithPlatform evidence={iconEvidence(r)} type="server">
        {opener(a.onOpen, r, r.assignedName || r.deviceName || r.ip)}
        <div className="flex items-center gap-2"><span className="font-mono text-[12px] text-ink-3">{r.ip}</span><ServiceIcons list={r.services} /></div>
        {(r.owner || r.department) && <div className="truncate text-[11.5px] text-ink-3">{[r.owner, r.department].filter(Boolean).join(' · ')}</div>}
      </WithPlatform>
    ) },
    { key: 'env', header: 'Environment', cell: (r) => <EnvironmentBadge name={r.environmentName} colour={r.environmentColour} /> },
    { key: 'critical', header: 'Critical', hideBelow: 'md', cell: (r) => (r.critical ? <Badge tone="danger">Critical</Badge> : none) },
    { key: 'check', header: 'Check', hideBelow: 'lg', cell: (r) => (r.monitor
      ? <Stack main={<span className="text-[12.5px] text-ink-2">{r.checkPort ? `Ping, then TCP ${r.checkPort}` : 'Ping'}</span>} sub={r.checkFailures ? `${r.checkFailures} failed in a row` : undefined} />
      : none) },
    { key: 'segment', header: 'Segment', hideBelow: 'lg', cell: segmentLink },
    { key: 'seen', header: 'On the network', hideBelow: 'md', cell: (r) => <Stack main={<SeenBadge lastSeen={r.lastSeen} />} sub={wrongMac(r) ? <span className="text-danger">Another MAC answers</span> : undefined} /> },
    seenAs,
    alerts,
    menu(a),
  ]
}

/** The whole register: what each address is for, with evidence beside it. */
export function registerColumns(a: Actions): Column<RegisterEntry>[] {
  return [
    { key: 'ip', header: 'Address', cell: (r) => <Stack mono main={opener(a.onOpen, r, r.ip, true)} sub={<span className="font-sans">{segmentLink(r)}</span>} /> },
    { key: 'status', header: 'Register', cell: (r) => <RegisterBadge status={r.status} /> },
    { key: 'name', header: 'Assigned to', cell: (r) => <WithPlatform evidence={iconEvidence(r)} type={r.ipType}><Stack main={r.assignedName || none} sub={label(ipTypes, r.ipType)} /></WithPlatform> },
    { key: 'owner', header: 'Owner', hideBelow: 'lg', cell: (r) => <Stack main={r.owner || none} sub={r.department || undefined} /> },
    { key: 'env', header: 'Environment', hideBelow: 'md', cell: (r) => <EnvironmentBadge name={r.environmentName} colour={r.environmentColour} /> },
    { key: 'mac', header: 'Expected MAC', hideBelow: 'xl', cell: (r) => (r.expectedMac
      ? <Stack mono main={r.expectedMac} sub={wrongMac(r) ? <span className="font-sans text-danger">Seen {r.seenMac}</span> : undefined} />
      : none) },
    { key: 'seen', header: 'On the network', hideBelow: 'md', cell: (r) => <SeenBadge lastSeen={r.lastSeen} /> },
    seenAs,
    alerts,
    menu(a),
  ]
}
