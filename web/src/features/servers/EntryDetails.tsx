import { Link } from 'react-router'
import { ServiceIcons } from '../../components/PlatformIcon'
import { Badge, Button, Details, EnvironmentBadge, HealthBadge, Modal, Notice, RegisterBadge, SeenBadge } from '../../components/ui'
import { ipTypes, label } from '../../lib/labels'
import { platformName, services } from '../../lib/platforms'
import type { RegisterEntry } from '../../lib/types'
import { EvidenceDetails, PortList, Section, when } from '../shared/DetailParts'
import { LinkedLinks } from '../links/LinkedLinks'

/** The one line that says whether the entry needs anyone. */
function Verdict({ r }: { r: RegisterEntry }) {
  const wrongMac = r.expectedMac && r.seenMac && r.expectedMac.toLowerCase() !== r.seenMac.toLowerCase()
  if (r.monitor && r.health === 'down') return <Notice tone="danger">Down{r.healthChangedAt ? ' since ' + new Date(r.healthChangedAt).toLocaleString('en-GB') : ''}. It failed {r.checkFailures} check{r.checkFailures === 1 ? '' : 's'} in a row.</Notice>
  if (wrongMac) return <Notice tone="danger">Another machine answers here: expected {r.expectedMac}, seen {r.seenMac}.</Notice>
  if (r.openAlerts > 0) {
    return <Notice tone="warn">{r.openAlerts} open alert{r.openAlerts === 1 ? '' : 's'} for this address. <Link className="font-medium underline" to={`/alerts?q=${encodeURIComponent(r.ip)}`}>Open alerts</Link></Notice>
  }
  if (r.status !== 'assigned' && r.lastSeen) return <Notice tone="warn">Marked {r.status}, but something answered here {new Date(r.lastSeen).toLocaleString('en-GB')}.</Notice>
  return <Notice tone="ok">{r.monitor ? (r.health === 'up' ? 'Up, and nothing else to do.' : 'Not checked yet.') : 'Nothing to do.'}</Notice>
}

/** Everything about one register entry: a server or any registered address. */
export function EntryDetails({ entry: r, canEdit, onClose, onEdit, onRelease }: {
  entry: RegisterEntry | null
  canEdit: boolean
  onClose: () => void
  onEdit: (r: RegisterEntry) => void
  onRelease: (r: RegisterEntry) => void
}) {
  if (!r) return null
  const act = (fn: (r: RegisterEntry) => void) => () => { onClose(); fn(r) }
  const wrongMac = r.expectedMac && r.seenMac && r.expectedMac.toLowerCase() !== r.seenMac.toLowerCase()
  return (
    <Modal open onClose={onClose} size="lg" title={`${r.ip} · ${r.assignedName || r.deviceName || 'Unnamed'}`}
      subtitle={<Link className="hover:text-accent hover:underline" to={`/segments/${r.segmentId}`} onClick={onClose}>{r.segmentName}</Link>}
      footer={<>
        {canEdit && <Button variant="danger" onClick={act(onRelease)}>Release address</Button>}
        {canEdit && <Button variant="primary" onClick={act(onEdit)}>Edit</Button>}
        <Button variant="ghost" onClick={onClose}>Close</Button>
      </>}>
      <div className="space-y-5">
        <Verdict r={r} />
        <Section title="Health check">
          <Details items={[
            ['Status', r.monitor ? <HealthBadge key="h" health={r.health} since={r.healthChangedAt} /> : 'Not monitored'],
            ['Changed', when(r.healthChangedAt)],
            ['Response time', r.monitor && r.health === 'up' && r.responseMs !== null ? `${r.responseMs} ms` : null],
            ['How', r.monitor ? (r.checkPort ? `Ping, then TCP port ${r.checkPort} when ping is blocked` : 'Ping') : null],
            ['Failures in a row', r.monitor ? String(r.checkFailures) : null],
            ['Critical', r.critical ? <Badge key="c" tone="danger">Critical</Badge> : 'No'],
            ['Environment', <EnvironmentBadge key="e" name={r.environmentName} colour={r.environmentColour} />],
          ]} />
        </Section>
        <Section title="What it is">
          <Details items={[
            ['Assigned to', r.assignedName || null],
            ['Device', r.deviceName],
            ['Type', label(ipTypes, r.ipType)],
            ['Platform', r.platform ? platformName(r.platform) : 'Not recorded'],
            ['Services', r.services.length ? <span key="sv" className="flex flex-wrap items-center gap-1.5"><ServiceIcons list={r.services} />{r.services.map((s) => services[s] ?? s).join(', ')}</span> : null],
            ['Owner', r.owner || null],
            ['Department', r.department || null],
          ]} />
        </Section>
        <Section title="What answers at this address">
          <div className="mb-2"><SeenBadge lastSeen={r.lastSeen} /></div>
          <EvidenceDetails e={r} mac={r.seenMac} />
          {r.expectedMac && <p className={`mt-2 text-[12.5px] ${wrongMac ? 'text-danger' : 'text-ink-3'}`}>Expected MAC: <span className="font-mono">{r.expectedMac}</span>{wrongMac ? ' (does not match)' : ''}</p>}
        </Section>
        <Section title="Open ports and services"><PortList ports={r.openPorts} checked={r.openPorts.length > 0 || Boolean(r.lastSeen)} /></Section>
        <Section title="Register">
          <Details items={[
            ['Status', <RegisterBadge key="s" status={r.status} />],
            ['Added', when(r.createdAt)],
            ['Last changed', when(r.updatedAt)],
          ]} />
          {r.notes && <p className="mt-3 rounded-md bg-surface-2 px-3 py-2 text-[13px] whitespace-pre-wrap text-ink-2">{r.notes}</p>}
        </Section>
        <LinkedLinks ip={r.ip} title={() => <h3 className="mb-2 text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase">Network links using this address</h3>} />
      </div>
    </Modal>
  )
}
