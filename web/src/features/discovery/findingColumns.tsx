import { AlertTriangle, CheckCircle2, Info, PlusCircle, ShieldCheck, XCircle } from 'lucide-react'
import { Link } from 'react-router'
import { WithPlatform } from '../../components/PlatformIcon'
import { Button, DeviceBadge, GatewayBadge, none, PrivateMacBadge, RegisterBadge, RowMenu, Stack, StateBadge, type Column } from '../../components/ui'
import { formatAgo, formatDateTime } from '../../lib/format'
import { label, sources } from '../../lib/labels'
import type { Finding } from '../../lib/types'
import { deviceLine, iconEvidence, networkCard, serviceName } from './deviceInfo'
import { OwnerName } from '../shared/OwnerName'
import { deviceDomain, windowsName } from './windowsInfo'

/** A MAC nobody has approved yet. Private MACs aren't devices to decide on. */
export const isUnknown = (r: { mac: string | null; isPrivateMac?: boolean | null; deviceStatus: string | null }) =>
  Boolean(r.mac) && !r.isPrivateMac && r.deviceStatus !== 'approved'

type RegisterFacts = { registerId: string | null; isGateway: boolean; isStatic: boolean | null; segmentKind?: string | null; inPool?: boolean; deviceStatus: string | null; homeIp?: string | null; source?: string | null }

/** A VPN client: its address comes from the FortiGate's VPN pool and changes each session, so it is never registered. */
export const isVpnClient = (r: Pick<RegisterFacts, 'source' | 'segmentKind'>) => r.source === 'vpn' || r.segmentKind === 'vpn'

/** The VPN kind in words: "SSL VPN", or "IPsec" with its tunnel. */
export const vpnLabel = (r: { vpnKind: string; vpnTunnel: string }) => (r.vpnKind === 'ssl' ? 'SSL VPN' : r.vpnKind === 'ipsec' ? `IPsec${r.vpnTunnel ? ` (${r.vpnTunnel})` : ''}` : 'VPN')

/**
 * An address the DHCP server leases: in a DHCP segment's pool. These are never
 * registered, as the address changes; the decision is approving the device.
 */
export const isDhcpClient = (r: Pick<RegisterFacts, 'isStatic' | 'segmentKind' | 'inPool'>) =>
  Boolean(r.segmentKind ? r.segmentKind === 'dhcp' : r.inPool) && !r.isStatic

/**
 * An address that should be in the register but isn't: a static address, or
 * any address on a server or cloud network. DHCP pool addresses never need it:
 * an unknown device there is flagged as unknown, and approved, not registered.
 */
export function needsRegister(r: RegisterFacts): boolean {
  if (r.registerId || r.isGateway || r.homeIp || isVpnClient(r)) return false
  return !isDhcpClient(r)
}

/** Row colour in Discovery: red for open alerts or a rejected device, orange for an unknown device or a missing register entry. */
export function attentionRow(r: Parameters<typeof needsRegister>[0] & { mac: string | null; isPrivateMac?: boolean | null; openAlerts?: number }): string | undefined {
  if ((r.openAlerts ?? 0) > 0 || r.deviceStatus === 'rejected') return 'bg-danger-soft/50 [&>td:first-child]:shadow-[inset_3px_0_0_var(--color-danger)]'
  if (isUnknown(r) || needsRegister(r)) return 'bg-warn-soft/50 [&>td:first-child]:shadow-[inset_3px_0_0_var(--color-warn)]'
  return undefined
}

/** First discovered within the last hour: a device that just joined. */
export const isNew = (firstSeen: string | null) => Boolean(firstSeen) && Date.now() - new Date(firstSeen!).getTime() < 3_600_000

/** A DHCP pool address: not registered, and not meant to be. */
export function DhcpNotRegistered() {
  return <StateBadge tone="neutral" title="DHCP addresses change, so they are not registered. Approve the device instead.">DHCP: not registered</StateBadge>
}

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
  if (!r.registerId && isVpnClient(r)) return <StateBadge tone="neutral" title="VPN addresses change with each session, so they are not registered.">VPN: not registered</StateBadge>
  if (!r.registerId && isDhcpClient(r)) return <DhcpNotRegistered />
  return <Stack main={<RegisterBadge status={r.registerStatus} attention={needsRegister(r)} />} sub={r.assignedName ?? undefined} />
}

/** First seen, last seen, and how long it has been silent once it stops answering. */
export function Presence({ online, firstSeen, lastSeen, showFirstSeen = true }: { online: boolean; firstSeen: string | null; lastSeen: string | null; showFirstSeen?: boolean }) {
  if (!lastSeen) return none
  return (
    <div className="min-w-0">
      {online
        ? <StateBadge tone="ok" title={`Last answered ${formatDateTime(lastSeen)}`}>Online</StateBadge>
        : <StateBadge tone="neutral" title={`Last answered ${formatDateTime(lastSeen)}`}>Not answering · last seen {formatAgo(lastSeen)}</StateBadge>}
      {showFirstSeen && firstSeen && <div className="mt-0.5 flex items-center gap-1.5 text-[11.5px] text-ink-3" title={formatDateTime(firstSeen)}>First discovered {formatAgo(firstSeen)}{isNew(firstSeen) && <StateBadge tone="accent">New</StateBadge>}</div>}
    </div>
  )
}

/** Name, what the device says about itself, its MAC and network card, and open ports. */
function DeviceCell({ r, onOpen }: { r: Finding; onOpen: (r: Finding) => void }) {
  const line = deviceLine(r)
  const card = networkCard(r)
  return (
    <WithPlatform evidence={iconEvidence(r)}>
      <div className="flex items-center gap-2 truncate text-ink">
        <button type="button" onClick={() => onOpen(r)} title="Show details" className={`cursor-pointer truncate text-left hover:text-accent hover:underline ${r.deviceStatus === 'approved' && r.deviceName ? 'font-medium' : ''}`}>
          <OwnerName owner={r.owner} name={r.deviceName || r.assignedName || r.hostname} fallback={r.isGateway ? 'Router' : r.source === 'vpn' ? `${vpnLabel(r)} client` : <span className="text-ink-3">No name found</span>} />
        </button>
        {r.isGateway && <GatewayBadge />}
      </div>
      {(r.deviceName || r.assignedName) && r.hostname && r.hostname !== (r.deviceName || r.assignedName) && <div className="truncate text-[11.5px] text-ink-3" title="Hostname the device reported">{r.hostname}</div>}
      <UserLine r={r} />
      {line && <div className="truncate text-[12px] text-ink-2" title="What the device says about itself">{line}</div>}
      {(deviceDomain(r) || r.osVersion) && <div className="truncate text-[11.5px] text-ink-3" title="Domain or workgroup, and Windows version, as the device reports them">{[deviceDomain(r) && `Domain: ${deviceDomain(r)}`, r.osVersion && windowsName(r.osVersion)].filter(Boolean).join(' · ')}</div>}
      <div className="flex items-center gap-2 truncate text-[11.5px] text-ink-3">
        <span className="font-mono">{r.mac ?? 'MAC not visible'}</span>{r.isPrivateMac && <PrivateMacBadge />}
        {card && <span className="truncate" title="Maker of the network card, from the MAC. Often not the device's maker.">· card: {card}</span>}
      </div>
      {r.openPorts.length > 0 && <div className="truncate text-[11.5px] text-ink-3" title={r.openPorts.map((p) => `${p} ${serviceName(p)}`).join(', ')}>Open: {r.openPorts.map(serviceName).join(', ')}</div>}
    </WithPlatform>
  )
}

/** Who uses the device: Intune's primary user, Entra ID's registered owner, or the user signed in to the VPN session. */
function UserLine({ r }: { r: Finding }) {
  if (r.intuneUser) {
    return <div className="truncate text-[12px] text-ink-2" title={`Intune primary user of ${r.intuneDevice || 'this device'}`}>User: <span className="text-ink">{r.intuneUserName || r.intuneUser}</span>{r.intuneUserName && <span className="text-ink-3"> · {r.intuneUser}</span>}</div>
  }
  if (r.entraUser) {
    return <div className="truncate text-[12px] text-ink-2" title={`Entra ID owner of ${r.entraDevice || 'this device'}, matched by computer name`}>User: <span className="text-ink">{r.entraUserName || r.entraUser}</span>{r.entraUserName && <span className="text-ink-3"> · {r.entraUser}</span>} <span className="text-ink-3">(Entra ID)</span></div>
  }
  if (r.vpnUser) {
    return <div className="truncate text-[12px] text-ink-2" title="User the FortiGate signed in to this VPN session">User: <span className="text-ink">{r.vpnUser}</span> <span className="text-ink-3">({vpnLabel(r)}{r.vpnRemote ? ` from ${r.vpnRemote}` : ''})</span></div>
  }
  return null
}

/** The one decision this row most needs, shown as a button instead of hidden in the menu. */
function mainAction(r: Finding, canEdit: boolean): 'approve' | 'register' | null {
  if (!canEdit) return null
  if (r.deviceId && r.deviceStatus === 'pending') return 'approve'
  if (!r.registerId && !isRoaming(r) && !isDhcpClient(r) && !isVpnClient(r) && r.segmentId && r.deviceStatus !== 'rejected' && !r.isGateway) return 'register'
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
    { key: 'device', header: 'Device', cell: (r) => <DeviceCell r={r} onOpen={onOpen} /> },
    { key: 'register', header: 'Register', cell: (r) => <RegisterCell r={r} /> },
    { key: 'approval', header: 'Approval', hideBelow: 'lg', cell: (r) => (r.deviceId || (r.mac && !r.isPrivateMac) ? <DeviceBadge status={r.deviceStatus} /> : none) },
    { key: 'presence', header: 'Presence', cell: (r) => <Presence online={r.online} firstSeen={r.firstSeen} lastSeen={r.lastSeen} /> },
    { key: 'source', header: 'Source', hideBelow: 'xl', cell: (r) => <Stack main={label(sources, r.source)} sub={r.source === 'vpn' ? vpnLabel(r) : r.segmentKind === 'dhcp' ? (r.isStatic ? 'Static (outside pool)' : 'DHCP pool') : undefined} /> },
    { key: 'alerts', header: '', className: 'w-8', cell: (r) => (r.openAlerts > 0
      ? <Link to={`/alerts?q=${encodeURIComponent(r.ip)}`} title={`${r.openAlerts} open alert${r.openAlerts > 1 ? 's' : ''}`} className="text-warn"><AlertTriangle size={15} aria-label="Open alerts" /></Link>
      : isKnown(r) ? <span title="Registered and approved: no warnings" className="text-ok"><ShieldCheck size={15} aria-label="Known device, no warnings" /></span> : null) },
    { key: 'actions', header: '', className: 'w-px whitespace-nowrap text-right', cell: (r) => (
      <div className="flex items-center justify-end gap-1">
      {mainAction(r, canEdit) === 'approve' && <Button className="h-7 px-2 text-[12px]" onClick={() => onApprove(r)}><CheckCircle2 size={13} aria-hidden />Approve</Button>}
      {mainAction(r, canEdit) === 'register' && <Button className="h-7 px-2 text-[12px]" onClick={() => onRegister(r)}><PlusCircle size={13} aria-hidden />Register</Button>}
      <RowMenu actions={[
        { label: 'Details', icon: <Info size={14} aria-hidden />, onSelect: () => onOpen(r) },
        { label: 'Add to register', icon: <PlusCircle size={14} aria-hidden />, onSelect: () => onRegister(r), hidden: !canEdit || Boolean(r.registerId) || !r.segmentId || isDhcpClient(r) || isVpnClient(r) },
        { label: 'Approve device', icon: <CheckCircle2 size={14} aria-hidden />, onSelect: () => onApprove(r), hidden: !canEdit || !r.deviceId || r.deviceStatus === 'approved' },
        { label: 'Reject device', icon: <XCircle size={14} aria-hidden />, onSelect: () => onReject(r), danger: true, hidden: !canEdit || !r.deviceId || r.deviceStatus !== 'pending' },
      ]} />
      </div>
    ) },
  ]
}
