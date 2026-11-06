import { useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { Link } from 'react-router'
import { BarList, LineChart } from '../../components/charts'
import { Badge, Button, Card, EmptyState, ErrorState, Meter, Mono, PageHeader, SkeletonRows, Td, Th } from '../../components/ui'
import { get, list } from '../../lib/api'
import { environments, formatCount, formatDateTime, humanAction } from '../../lib/format'
import { useCan, useMe } from '../../lib/session'
import { LimitedDashboard } from './LimitedDashboard'
import type { AuditEntry, Distribution, Subnet, Summary, TrendPoint } from '../../lib/types'

const tiles = 'grid grid-cols-[repeat(auto-fit,minmax(10.5rem,1fr))] divide-x divide-line overflow-hidden rounded-md border border-line bg-surface'

function Stat({ label, value, to, note, alert }: { label: string; value: number; to?: string; note?: string; alert?: boolean }) {
  const body: ReactNode = (
    <>
      <div className="text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase">{label}</div>
      <div className={`mt-1 text-[24px] leading-none font-semibold tracking-tight ${alert && value > 0 ? 'text-danger' : 'text-ink'}`}>{formatCount(value)}</div>
      {note && <div className="mt-1 text-[11.5px] text-ink-3">{note}</div>}
    </>
  )
  const box = 'block min-w-0 px-4 py-3'
  return to ? <Link to={to} className={`${box} transition-colors hover:bg-surface-2`}>{body}</Link> : <div className={box}>{body}</div>
}

const states = [
  { key: 'allocated', label: 'Allocated', color: 'var(--state-allocated)' },
  { key: 'reserved', label: 'Reserved', color: 'var(--state-reserved)' },
  { key: 'availableCandidates', label: 'Available candidate', color: 'var(--state-available)' },
  { key: 'quarantined', label: 'Quarantined', color: 'var(--state-quarantined)' },
] as const

function AllocationBar({ s }: { s: Summary }) {
  const total = states.reduce((sum, st) => sum + s[st.key], 0)
  if (total === 0) return <EmptyState title="No IP records yet" body="Allocation state appears here once addresses are tracked." />
  const describe = states.map((st) => `${st.label} ${formatCount(s[st.key])}`).join(', ')
  return (
    <div className="p-4">
      <div role="img" aria-label={`Allocation state: ${describe}`} className="flex h-7 gap-0.5">
        {states.filter((st) => s[st.key] > 0).map((st) => {
          const share = (s[st.key] / total) * 100
          return (
            <div key={st.key} title={`${st.label}: ${formatCount(s[st.key])} (${share.toFixed(1)}%)`}
              className="min-w-1 rounded" style={{ width: `${share}%`, background: st.color }} />
          )
        })}
      </div>
      <table className="mt-3 w-full text-[13px]">
        <caption className="sr-only">Allocation state counts</caption>
        <tbody>
          {states.map((st) => (
            <tr key={st.key}>
              <td className="py-1"><span className="mr-2 inline-block size-2.5 rounded-sm align-baseline" style={{ background: st.color }} />{st.label}</td>
              <td className="py-1 text-right text-ink-2 tabular-nums">{formatCount(s[st.key])}</td>
              <td className="w-16 py-1 text-right text-ink-3 tabular-nums">{((s[st.key] / total) * 100).toFixed(1)}%</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function BusiestSubnets() {
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['subnets', 'busiest'], queryFn: () => list<Subnet>('/subnets?pageSize=8&sort=-utilisation') })
  if (isPending) return <SkeletonRows rows={4} />
  if (error) return <ErrorState error={error} onRetry={refetch} />
  if (data.data.length === 0) {
    return <EmptyState title="No subnets yet" body="Add your first subnet to start tracking utilisation." action={<Link to="/subnets"><Button variant="primary">Go to subnets</Button></Link>} />
  }
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse">
        <thead><tr><Th>Subnet</Th><Th>CIDR</Th><Th>Environment</Th><Th>Utilisation</Th></tr></thead>
        <tbody>
          {data.data.map((s) => {
            const pct = Number(s.utilisationPct)
            return (
              <tr key={s.id}>
                <Td><Link to={`/subnets?q=${encodeURIComponent(s.name)}`} className="font-medium text-ink hover:text-accent">{s.name}</Link></Td>
                <Td><Mono>{s.calculated.cidr}</Mono></Td>
                <Td><Badge>{environments[s.environment]}</Badge></Td>
                <Td><Meter pct={pct} level={s.utilisationLevel} label={`${s.name} utilisation`} /></Td>
              </tr>
            )
          })}
        </tbody>
      </table>
    </div>
  )
}

function Trends() {
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['dashboard', 'trends'], queryFn: () => get<TrendPoint[]>('/dashboard/trends?days=30') })
  if (isPending) return <Card><SkeletonRows rows={5} /></Card>
  if (error) return <Card><ErrorState error={error} onRetry={refetch} /></Card>
  const utilisation = data.map((p) => ({ label: p.date, value: p.averagePct === '' ? null : Number(p.averagePct) }))
  return (
    <div className="grid gap-4 xl:grid-cols-3">
      <Card title="Average subnet utilisation, 30 days">
        <LineChart title="Average subnet utilisation" points={utilisation} unit="%" emptyBody="The worker records a snapshot once a day." />
      </Card>
      <Card title="New findings, 30 days">
        <LineChart title="New findings" points={data.map((p) => ({ label: p.date, value: p.newFindings }))} emptyBody="Findings appear after reconciliation runs." />
      </Card>
      <Card title="Evidence collected, 30 days">
        <LineChart title="Observations collected" points={data.map((p) => ({ label: p.date, value: p.observations }))} emptyBody="Evidence appears after discovery or an integration sync." />
      </Card>
    </div>
  )
}

function Breakdown() {
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['dashboard', 'distribution'], queryFn: () => get<Distribution>('/dashboard/distribution') })
  if (isPending) return <Card><SkeletonRows rows={5} /></Card>
  if (error) return <Card><ErrorState error={error} onRetry={refetch} /></Card>
  return (
    <div className="grid gap-4 md:grid-cols-3">
      <Card title="Subnets by environment"><BarList title="Subnets by environment" items={data.environments.map((s) => ({ ...s, label: environments[s.label] ?? s.label }))} /></Card>
      <Card title="Subnets by site"><BarList title="Subnets by site" items={data.sites} /></Card>
      <Card title="IPv4 and IPv6 subnets"><BarList title="Subnets by address family" items={data.families} /></Card>
    </div>
  )
}

function RecentChanges() {
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['audit', 'recent'], queryFn: () => list<AuditEntry>('/audit-logs?pageSize=6') })
  if (isPending) return <SkeletonRows rows={4} />
  if (error) return <ErrorState error={error} onRetry={refetch} />
  if (data.data.length === 0) return <EmptyState title="No changes recorded yet" />
  return (
    <ul>
      {data.data.map((e) => (
        <li key={e.id} className="border-b border-line px-4 py-2 last:border-0">
          <div className="text-[13px] font-medium text-ink">{humanAction(e.action)}</div>
          <div className="text-xs text-ink-3">{e.actorEmail ?? e.actorType} · {formatDateTime(e.occurredAt)}</div>
        </li>
      ))}
    </ul>
  )
}

/** Inventory, shaped by what this role can act on. Tiles only link to pages it can open. */
function InventoryDashboard() {
  const { permissions, roles } = useMe()
  const has = (...any: string[]) => any.some((p) => permissions.includes(p))
  const canAudit = has('audit:view')
  const seesIntegrations = has('integrations:manage', 'audit:view')
  const seesDiscovery = has('discovery:run', 'discovery:manage', 'audit:view')
  const seesRequests = has('ip:request', 'requests:approve', 'ip:allocate', 'ip:reserve', 'audit:view')
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['summary'], queryFn: () => get<Summary>('/dashboard/summary') })
  return (
    <>
      <PageHeader title="Dashboard" description={`Address inventory across all routing domains. Shown for ${roles.join(', ')}.`} />
      {isPending && <Card><SkeletonRows /></Card>}
      {error && <Card><ErrorState error={error} onRetry={refetch} /></Card>}
      {data && (
        <div className="space-y-3">
          <div className={tiles}>
            <Stat label="IPv4 subnets" value={data.subnetsV4} to="/subnets?family=4" />
            <Stat label="IPv6 subnets" value={data.subnetsV6} to="/subnets?family=6" />
            <Stat label="Tracked IP records" value={data.ipRecords} to="/ip-addresses/all" />
            <Stat label="Observed active" value={data.observedActive} note="Reachability, not allocation" />
            <Stat label="Open findings" value={data.openFindings} to="/reconciliation" alert />
            <Stat label="Devices" value={data.devices} to="/devices" />
          </div>
          <div className={tiles}>
            {has('requests:approve') && <Stat label="Pending approvals" value={data.pendingApprovals ?? 0} to="/requests/approvals" alert />}
            {has('ip:allocate', 'ip:reserve') && <Stat label="Approved, waiting to be fulfilled" value={data.readyToFulfil ?? 0} to="/requests/all" />}
            <Stat label="Reservations ending within 7 days" value={data.reservationsExpiringSoon} to="/reservations" alert />
            {!has('requests:approve', 'ip:allocate', 'ip:reserve') && seesRequests && <Stat label="Pending approvals" value={data.pendingApprovals ?? 0} to={canAudit ? '/requests/all' : '/requests'} />}
          </div>
          <div className={tiles}>
            <Stat label="High-utilisation subnets" value={data.highUtilisationSubnets} to="/subnets" alert note="At the latest snapshot" />
            <Stat label="Suspected conflicts" value={data.conflictSuspected} to="/reconciliation" alert />
            {seesIntegrations && <Stat label="Integrations healthy" value={data.integrationsHealthy ?? 0} to="/integrations" />}
            {seesIntegrations && <Stat label="Integrations failing" value={data.integrationsFailing ?? 0} to="/integrations" alert />}
            {seesDiscovery && <Stat label="Agents online" value={data.agentsOnline ?? 0} to="/discovery/agents" note={data.agentsOffline ? `${formatCount(data.agentsOffline)} offline` : undefined} />}
            {seesDiscovery && <Stat label="Discovery jobs failed today" value={data.discoveryJobsFailedToday ?? 0} to="/discovery" alert note={`${formatCount(data.discoveryJobsRunning ?? 0)} running`} />}
          </div>
          <Trends />
          <Breakdown />
          <div className="grid gap-4 xl:grid-cols-3">
            <Card title="Most utilised subnets" className="xl:col-span-2"><BusiestSubnets /></Card>
            <div className="space-y-4">
              <Card title="Allocation state"><AllocationBar s={data} /></Card>
              {canAudit && <Card title="Recent changes" action={<Link to="/audit" className="text-[13px] text-accent hover:underline">View all</Link>}><RecentChanges /></Card>}
            </div>
          </div>
        </div>
      )}
    </>
  )
}

export function Dashboard() {
  return useCan('inventory:view') ? <InventoryDashboard /> : <LimitedDashboard />
}
