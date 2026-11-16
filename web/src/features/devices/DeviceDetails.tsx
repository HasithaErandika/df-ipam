import { Link } from 'react-router'
import { Button, Details, DeviceBadge, Modal, Notice, StateBadge } from '../../components/ui'
import { deviceTypes, label } from '../../lib/labels'
import { platformName } from '../../lib/platforms'
import type { Device } from '../../lib/types'
import { EvidenceDetails, PortList, Section, when } from '../shared/DetailParts'

/** The one line that says whether the device needs anyone. */
function Verdict({ d }: { d: Device }) {
  if (d.status === 'rejected') return <Notice tone="danger">Rejected. It raises a High alert whenever it is connected.</Notice>
  if (d.openAlerts > 0) {
    return <Notice tone="warn">{d.openAlerts} open alert{d.openAlerts === 1 ? '' : 's'} for this device. <Link className="font-medium underline" to={`/alerts?q=${encodeURIComponent(d.mac)}`}>Open alerts</Link></Notice>
  }
  if (d.status === 'pending') return <Notice>Nobody has decided on this device yet. Approve or reject it.</Notice>
  return <Notice tone="ok">Approved{d.registeredIp ? `, and registered at ${d.registeredIp}` : ''}. Nothing to do.</Notice>
}

/** Everything known about one device, with the decisions a person can take. */
export function DeviceDetails({ device: d, canEdit, onClose, onApprove, onReject, onEdit }: {
  device: Device | null
  canEdit: boolean
  onClose: () => void
  onApprove: (d: Device) => void
  onReject: (d: Device) => void
  onEdit: (d: Device) => void
}) {
  if (!d) return null
  const act = (fn: (d: Device) => void) => () => { onClose(); fn(d) }
  return (
    <Modal open onClose={onClose} size="lg" title={d.name || d.hostname || d.mac} subtitle={<span className="font-mono">{d.mac}</span>}
      footer={<>
        {canEdit && d.status === 'pending' && <Button variant="danger" onClick={act(onReject)}>Reject</Button>}
        {canEdit && d.status !== 'approved' && <Button variant="primary" onClick={act(onApprove)}>Approve</Button>}
        {canEdit && <Button onClick={act(onEdit)}>Edit</Button>}
        <Button variant="ghost" onClick={onClose}>Close</Button>
      </>}>
      <div className="space-y-5">
        <Verdict d={d} />
        <Section title="The device">
          <EvidenceDetails e={d} mac={d.mac} privateMac={d.isPrivateMac} />
        </Section>
        <Section title="On the network">
          <Details items={[
            ['Now', d.online ? <StateBadge tone="ok">Online</StateBadge> : <StateBadge tone="neutral">Not seen in the last 30 minutes</StateBadge>],
            ['Last seen address', d.lastIp ? <span><span className="font-mono">{d.lastIp}</span><span className="block text-ink-3">Observed by discovery; may change with DHCP.</span></span> : null],
            ['Segment', d.segmentId ? <Link className="hover:text-accent hover:underline" to={`/segments/${d.segmentId}`} onClick={onClose}>{d.segmentName}</Link> : null],
            ['Addresses seen', d.addresses ? `${d.addresses}${d.addresses > 1 ? ' (it has moved between addresses or networks)' : ''}` : null],
            ['Register entry', d.registeredIp ? <span className="font-mono">{d.registeredIp}</span> : d.registered ? 'Its last address is registered' : 'Not registered'],
            ['Seen by', d.source || null],
            ['First seen', when(d.firstSeen)],
            ['Last seen', when(d.lastSeen)],
          ]} />
        </Section>
        <Section title="Open ports and services"><PortList ports={d.openPorts} checked={d.openPorts.length > 0 || d.online} /></Section>
        <Section title="Ownership and decision">
          <Details items={[
            ['Status', <DeviceBadge key="s" status={d.status} />],
            ['Decided', when(d.decidedAt)],
            ['Type', label(deviceTypes, d.deviceType)],
            ['Platform', d.platform ? platformName(d.platform) : 'Not recorded'],
            ['Owner', d.owner || null],
            ['Department', d.department || null],
          ]} />
          {d.notes && <p className="mt-3 rounded-md bg-surface-2 px-3 py-2 text-[13px] whitespace-pre-wrap text-ink-2">{d.notes}</p>}
        </Section>
      </div>
    </Modal>
  )
}
