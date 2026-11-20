import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Link } from 'react-router'
import { Card, DataTable, EmptyState, ErrorState, none, Notice, SkeletonRows, Stat, StateBadge, Switch, type Column } from '../../components/ui'
import { ApiError, get } from '../../lib/api'
import { formatAgo, formatCount } from '../../lib/format'
import type { FirewallLinkRef, FirewallLive } from '../../lib/types'
import { formatRate, share } from './format'

type Iface = FirewallLive['interfaces'][number]
type Tunnel = FirewallLive['tunnels'][number]
type Health = FirewallLive['health'][number]
type VPNUser = FirewallLive['vpnUsers'][number]

/** True when the answer means no firewall is connected, rather than a failure. */
export const notConnected = (e: unknown) => e instanceof ApiError && e.code === 'FIREWALL_NOT_CONNECTED'

/**
 * The live firewall data, shared by the Firewall page, each link's page and the
 * monitor. Without a connected firewall it stops asking instead of every 10 seconds.
 */
export const useFirewallLive = (enabled = true) => useQuery({
  queryKey: ['/network-links', 'firewall', 'live'], queryFn: () => get<FirewallLive>('/firewall/live'),
  refetchInterval: (q) => (notConnected(q.state.error) ? false : 10_000), enabled, retry: false,
})

/** A rate with a bar against the link's bandwidth, when the link has one. */
export function RateCell({ bps, of }: { bps: number | null; of?: number | null }) {
  const pct = share(bps, of)
  return (
    <div className="min-w-24">
      <div className="tabular-nums">{formatRate(bps)}</div>
      {pct != null && <div className="mt-0.5 h-1.5 rounded-full bg-track" title={`${pct.toFixed(0)}% of ${of} Mbps`}>
        <div className={`h-1.5 rounded-full ${pct > 85 ? 'bg-danger' : pct > 60 ? 'bg-warn' : 'bg-accent'}`} style={{ width: `${Math.max(pct, 2)}%` }} /></div>}
    </div>
  )
}

function LinkCell({ link }: { link: FirewallLinkRef | null }) {
  return link
    ? <Link to={`/network-links/${link.id}`} className="text-accent hover:underline"><span className="font-mono text-[12px]">{link.code}</span> {link.name}</Link>
    : <span className="text-ink-3">Not recorded</span>
}

const ifaceColumns: Column<Iface>[] = [
  { key: 'name', header: 'Interface', cell: (i) => <div><div className="font-medium text-ink">{i.name}</div>{i.alias && <div className="text-[12px] text-ink-3">{i.alias}</div>}</div> },
  { key: 'status', header: 'Status', cell: (i) => <StateBadge tone={i.up ? 'ok' : 'danger'}>{i.up ? 'Up' : 'Down'}</StateBadge> },
  { key: 'ip', header: 'Address', hideBelow: 'md', cell: (i) => (i.ip && i.ip !== '0.0.0.0' ? <span className="font-mono text-[12.5px]">{i.ip}</span> : none) },
  { key: 'role', header: 'Role', hideBelow: 'lg', cell: (i) => i.role ? i.role.toUpperCase() : none },
  { key: 'down', header: 'Download', cell: (i) => <RateCell bps={i.rxBps} of={i.link?.downMbps ?? (i.speedMbps || null)} /> },
  { key: 'up', header: 'Upload', cell: (i) => <RateCell bps={i.txBps} of={i.link?.upMbps ?? (i.speedMbps || null)} /> },
  { key: 'speed', header: 'Port speed', hideBelow: 'xl', cell: (i) => (i.speedMbps ? `${formatCount(i.speedMbps)} Mbps` : none) },
  { key: 'errors', header: 'Errors', hideBelow: 'xl', className: 'tabular-nums', cell: (i) => (i.errors ? <span className="text-warn">{formatCount(i.errors)}</span> : '0') },
  { key: 'link', header: 'Network link', cell: (i) => <LinkCell link={i.link} /> },
]

const tunnelColumns: Column<Tunnel>[] = [
  { key: 'name', header: 'Tunnel', cell: (t) => <div><div className="font-medium text-ink">{t.name}</div>{t.comments && <div className="text-[12px] text-ink-3">{t.comments}</div>}</div> },
  { key: 'status', header: 'Status', cell: (t) => <StateBadge tone={t.phaseUp === 0 ? 'danger' : t.phaseUp < t.phases ? 'warn' : 'ok'}>{t.phaseUp === 0 ? 'Down' : `Up ${t.phaseUp}/${t.phases}`}</StateBadge> },
  { key: 'gw', header: 'Remote gateway', hideBelow: 'md', cell: (t) => (t.gateway ? <span className="font-mono text-[12.5px]">{t.gateway}</span> : none) },
  { key: 'in', header: 'In', cell: (t) => <RateCell bps={t.rxBps} /> },
  { key: 'out', header: 'Out', cell: (t) => <RateCell bps={t.txBps} /> },
  { key: 'link', header: 'Network link', cell: (t) => <LinkCell link={t.link} /> },
]

const vpnColumns: Column<VPNUser>[] = [
  { key: 'user', header: 'User', cell: (u) => <span className="font-medium text-ink">{u.user || '—'}</span> },
  { key: 'kind', header: 'VPN', cell: (u) => (u.kind === 'ssl' ? 'SSL VPN' : `IPsec${u.tunnel ? ` · ${u.tunnel}` : ''}`) },
  { key: 'ip', header: 'Tunnel address', cell: (u) => (u.tunnelIp ? <span className="font-mono text-[12.5px]">{u.tunnelIp}</span> : none) },
  { key: 'from', header: 'Connected from', hideBelow: 'md', cell: (u) => (u.from ? <span className="font-mono text-[12.5px]">{u.from}</span> : none) },
  { key: 'since', header: 'Since', hideBelow: 'lg', cell: (u) => u.since || none },
]

const healthColumns: Column<Health>[] = [
  { key: 'check', header: 'Health check', cell: (h) => h.check },
  { key: 'member', header: 'Member', cell: (h) => h.member },
  { key: 'status', header: 'Status', cell: (h) => <StateBadge tone={h.status === 'up' ? 'ok' : 'danger'}>{h.status || 'unknown'}</StateBadge> },
  { key: 'latency', header: 'Latency', className: 'tabular-nums', cell: (h) => `${h.latencyMs.toFixed(1)} ms` },
  { key: 'jitter', header: 'Jitter', className: 'tabular-nums', cell: (h) => `${h.jitterMs.toFixed(1)} ms` },
  { key: 'loss', header: 'Packet loss', className: 'tabular-nums', cell: (h) => <span className={h.lossPct > 1 ? 'text-danger' : undefined}>{h.lossPct.toFixed(1)}%</span> },
]

/** What the firewall says right now, refreshed every 10 seconds. Nothing is stored. */
export function LiveView() {
  const q = useFirewallLive()
  const [all, setAll] = useState(false)
  if (q.isPending) return <Card><SkeletonRows rows={8} /></Card>
  if (q.isError) {
    return <Card>{notConnected(q.error)
      ? <EmptyState title="No firewall connected" body="An administrator connects the FortiGate in System > FortiGate, with its address and a read-only API token whose profile can read System, Network and VPN." />
      : <ErrorState error={q.error} onRetry={() => void q.refetch()} />}</Card>
  }
  const v = q.data
  const ifaces = all ? v.interfaces : v.interfaces.filter((i) => i.link || i.role === 'wan')
  const pct = (n: number | null) => (n == null ? '—' : `${n}%`)
  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3 md:grid-cols-4">
        <Stat label="Firewall" value={<span className="text-[15px]">{v.firewall}</span>} hint={`Updated ${formatAgo(v.at)}, every 10 seconds`} />
        <Stat label="CPU" value={pct(v.cpu)} tone={v.cpu != null && v.cpu > 80 ? 'danger' : undefined} />
        <Stat label="Memory" value={pct(v.memory)} tone={v.memory != null && v.memory > 85 ? 'danger' : undefined} />
        <Stat label="Sessions" value={v.sessions == null ? '—' : formatCount(v.sessions)} />
      </div>
      {v.problems.length > 0 && <Notice tone="warn">Not everything could be read: {v.problems.join(' · ')}. Check the API token's admin profile.</Notice>}
      <Card title="Interfaces" subtitle="Download is traffic the firewall receives on the interface. Bars compare with the link's bandwidth, or the port speed."
        action={<Switch id="fw-all" checked={all} onChange={setAll} label="All interfaces" />}>
        <DataTable columns={ifaceColumns} rows={ifaces} query={q} rowKey={(i) => i.name}
          empty={{ title: 'No WAN interfaces', body: 'Turn on All interfaces to see the rest.' }} />
      </Card>
      <Card title="VPN users" subtitle={`${v.vpnUsers.length} connected now over SSL or dial-up IPsec`}>
        <DataTable columns={vpnColumns} rows={v.vpnUsers} query={q} rowKey={(u) => u.kind + u.user + u.tunnelIp} empty={{ title: 'Nobody is connected over VPN' }} />
      </Card>
      <Card title="IPsec tunnels">
        <DataTable columns={tunnelColumns} rows={v.tunnels} query={q} rowKey={(t) => t.name} empty={{ title: 'No IPsec tunnels on this firewall' }} />
      </Card>
      {v.health.length > 0 && (
        <Card title="SD-WAN health" subtitle="The firewall's own measurements to its health-check servers.">
          <DataTable columns={healthColumns} rows={v.health} query={q} rowKey={(h) => h.check + h.member} empty={{ title: 'No health checks' }} />
        </Card>
      )}
    </div>
  )
}
