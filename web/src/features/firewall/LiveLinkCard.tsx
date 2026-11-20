import { Link } from 'react-router'
import { Card, Details, StateBadge } from '../../components/ui'
import { formatAgo } from '../../lib/format'
import type { NetworkLink } from '../../lib/types'
import { RateCell, useFirewallLive } from './LiveView'

/**
 * The link as the firewall sees it now, when an interface or tunnel matches it.
 * Shows nothing when no firewall is connected or nothing matches.
 */
export function LiveLinkCard({ link }: { link: NetworkLink }) {
  const relevant = Boolean(link.interface || link.vpnTunnel || link.wanIp)
  const q = useFirewallLive(relevant && !link.archivedAt)
  if (!q.data) return null
  const iface = q.data.interfaces.find((i) => i.link?.id === link.id)
  const tunnel = q.data.tunnels.find((t) => t.link?.id === link.id)
  if (!iface && !tunnel) return null
  const status = iface
    ? <StateBadge tone={iface.up ? 'ok' : 'danger'}>{iface.up ? 'Up' : 'Down'}</StateBadge>
    : <StateBadge tone={tunnel!.phaseUp === 0 ? 'danger' : 'ok'}>{tunnel!.phaseUp === 0 ? 'Down' : `Up ${tunnel!.phaseUp}/${tunnel!.phases}`}</StateBadge>
  return (
    <Card title="Live from the firewall" subtitle={`${q.data.firewall}, updated ${formatAgo(q.data.at)}`}
      action={<Link to="/firewall/live" className="text-[12.5px] text-accent hover:underline">Firewall</Link>}>
      <div className="p-4">
        <Details items={[
          ['Status', status],
          [iface ? 'Interface' : 'Tunnel', <span key="n" className="font-mono">{iface?.name ?? tunnel?.name}</span>],
          ['Download', <RateCell key="d" bps={iface ? iface.rxBps : tunnel!.rxBps} of={link.downMbps} />],
          ['Upload', <RateCell key="u" bps={iface ? iface.txBps : tunnel!.txBps} of={link.upMbps} />],
        ]} />
      </div>
    </Card>
  )
}
