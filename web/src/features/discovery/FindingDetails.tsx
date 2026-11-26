import { type ReactNode } from 'react'
import { Link } from 'react-router'
import { Button, Details, DeviceBadge, GatewayBadge, Modal, Notice, PrivateMacBadge, RegisterBadge } from '../../components/ui'
import { formatAgo, formatDateTime } from '../../lib/format'
import { label, sources } from '../../lib/labels'
import { likelyPlatform, platformName } from '../../lib/platforms'
import type { Finding } from '../../lib/types'
import { deviceFacts, networkCard, serviceName } from './deviceInfo'
import { hasManagement, ManagementSection } from './ManagementSection'
import { DhcpNotRegistered, isDhcpClient, isKnown, isRoaming, isVpnClient, Presence, RegisterCell, vpnLabel } from './findingColumns'

const when = (t: string | null) => (t ? <span title={formatDateTime(t)}>{formatDateTime(t)} ({formatAgo(t)})</span> : null)

function Section({ title, children }: { title: string; children: ReactNode }) {
  return <section><h3 className="mb-2 text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase">{title}</h3>{children}</section>
}

/** What tells a person whether this address needs them, in one line. */
function Verdict({ r }: { r: Finding }) {
  if (r.openAlerts > 0) {
    return <Notice tone="warn">{r.openAlerts} open alert{r.openAlerts > 1 ? 's' : ''} for this address. <Link className="font-medium underline" to={`/alerts?q=${encodeURIComponent(r.ip)}`}>Open alerts</Link></Notice>
  }
  if (r.deviceStatus === 'rejected') return <Notice tone="danger">This device was rejected. It raises a High alert while it is connected.</Notice>
  if (isKnown(r) && isRoaming(r)) {
    return <Notice tone="ok">Known device on another network: it is registered at <span className="font-mono">{r.homeIp}</span> on {r.homeSegment} and approved. Nothing to do.</Notice>
  }
  if (isKnown(r)) return <Notice tone="ok">Known: the address is in the register{r.mac ? ' and the device is approved' : ''}. Nothing to do.</Notice>
  if (r.deviceId && r.deviceStatus !== 'approved') return <Notice>Nobody has decided on this device yet. Approve or reject it.</Notice>
  if (isVpnClient(r)) {
    return r.vpnUser
      ? <Notice tone="ok">VPN session of {r.vpnUser}, signed in by the FortiGate. Nothing to do.</Notice>
      : <Notice>VPN session with no user name from the FortiGate. Check the tunnel if you don't expect it.</Notice>
  }
  if (!r.registerId && isDhcpClient(r)) return <Notice>On a DHCP address, which changes, so it isn't registered. Nothing to do.</Notice>
  if (!r.registerId) return <Notice>Seen on the network, not in the register. Register it if it should keep this address.</Notice>
  return null
}

/** Everything known about one finding, with the decisions a person can take on it. */
export function FindingDetails({ row, canEdit, onClose, onRegister, onApprove, onReject }: {
  row: Finding | null
  canEdit: boolean
  onClose: () => void
  onRegister: (r: Finding) => void
  onApprove: (r: Finding) => void
  onReject: (r: Finding) => void
}) {
  if (!row) return null
  const r = row
  const act = (fn: (r: Finding) => void) => () => { onClose(); fn(r) }
  const f = deviceFacts(r)
  const card = networkCard(r)
  const guess = likelyPlatform(r)
  const name = r.deviceName || r.assignedName || r.hostname || (r.isGateway ? 'Router' : 'Unnamed device')
  return (
    <Modal open onClose={onClose} size="lg" title={`${r.ip} · ${name}`}
      subtitle={r.segmentId ? <Link className="hover:text-accent hover:underline" to={`/segments/${r.segmentId}`} onClick={onClose}>{r.segmentName}</Link> : 'No segment'}
      footer={<>
        {canEdit && !r.registerId && r.segmentId && !isDhcpClient(r) && !isVpnClient(r) && <Button onClick={act(onRegister)}>Add to register</Button>}
        {canEdit && r.deviceId && r.deviceStatus === 'pending' && <Button variant="danger" onClick={act(onReject)}>Reject device</Button>}
        {canEdit && r.deviceId && r.deviceStatus !== 'approved' && <Button variant="primary" onClick={act(onApprove)}>Approve device</Button>}
        <Button variant="ghost" onClick={onClose}>Close</Button>
      </>}>
      <div className="space-y-5">
        <Verdict r={r} />
        <Section title="The device">
          <Details items={[
            ['Maker', f.maker ? <span title={f.makerFrom}>{f.maker} <span className="text-ink-3">({f.makerFrom.toLowerCase()})</span></span> : null],
            ['Model', f.model || null],
            ['Kind', r.isGateway ? <GatewayBadge /> : f.kind || null],
            ['Likely system', f.system ? <span title="From the ping reply's starting TTL. A hint, not proof.">{f.system}</span> : null],
            ['Platform', r.platform ? platformName(r.platform) : guess ? <span className="text-ink-3">Not recorded (likely {platformName(guess)})</span> : 'Not recorded'],
            ['Hostname', r.hostname || null],
            ...(f.dhcpClass ? [['DHCP vendor class', f.dhcpClass] as [string, string]] : []),
          ]} />
          {!f.maker && !f.model && !f.kind && <p className="mt-2 text-[12.5px] text-ink-3">It hasn't said what it is. Phones, PCs and laptops rarely do unless they answer mDNS or UPnP; Apple devices and TVs, printers and routers usually do.</p>}
        </Section>
        {(hasManagement(r) || (!r.isGateway && r.source !== 'vpn' && r.mac && !r.isPrivateMac)) && (
          <Section title="Windows, Intune and Entra ID">
            <ManagementSection r={r} />
          </Section>
        )}
        <Section title="Open ports and services">
          {!r.portsChecked
            ? <p className="text-[12.5px] text-ink-3">Not checked on this segment. An Admin can turn on "Check common ports" in the segment's settings.</p>
            : r.openPorts.length === 0
              ? <p className="text-[12.5px] text-ink-3">None of the common ports answered in the last scan.</p>
              : <ul className="flex flex-wrap gap-1.5">{r.openPorts.map((p) => (
                  <li key={p} className="inline-flex items-center gap-1.5 rounded-[4px] border border-line bg-surface-2 px-2 py-0.5 text-[12.5px]">
                    <span className="font-mono text-ink">{p}</span><span className="text-ink-2">{serviceName(p)}</span>
                  </li>
                ))}</ul>}
        </Section>
        <Section title="Network card">
          <Details items={[
            ['MAC address', r.mac ? <span className="flex items-center gap-2 font-mono">{r.mac}{r.isPrivateMac && <PrivateMacBadge />}</span> : 'Not visible to the scanner'],
            ['Card maker', card ? <span title="From the MAC's first half. It names the card, not the device.">{card}</span> : r.isPrivateMac ? 'Hidden: the device uses a private (random) MAC' : null],
          ]} />
        </Section>
        <Section title="In DF-Atlas">
          <Details items={[
            ['Presence', <Presence key="p" online={r.online} firstSeen={r.firstSeen} lastSeen={r.lastSeen} />],
            ['Register', isRoaming(r) || (!r.registerId && isVpnClient(r)) ? <RegisterCell key="r" r={r} /> : !r.registerId && isDhcpClient(r) ? <DhcpNotRegistered key="r" /> : <span key="r" className="flex items-center gap-2"><RegisterBadge status={r.registerStatus} />{r.assignedName}</span>],
            ['Device approval', r.deviceId || (r.mac && !r.isPrivateMac) ? <DeviceBadge status={r.deviceStatus} /> : 'Not tracked (private MAC on a segment that does not watch them)'],
            ['Owner', r.owner],
            ['Seen by', label(sources, r.source)],
            ...(r.source === 'vpn' ? [['VPN', <span key="v">{r.vpnUser || 'No user name'} · {vpnLabel(r)}{r.vpnRemote ? <> from <span className="font-mono">{r.vpnRemote}</span></> : null}</span>] as [string, ReactNode]] : []),
            ['Address type', isVpnClient(r) ? 'VPN client address (changes each session)' : r.segmentKind === 'dhcp' ? (r.isStatic ? 'Static (outside the DHCP pool)' : 'DHCP pool') : 'Static'],
            ['First discovered', when(r.firstSeen)],
            ['Last answered', when(r.lastSeen)],
          ]} />
        </Section>
      </div>
    </Modal>
  )
}
