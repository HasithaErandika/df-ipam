import { useQuery } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { Link } from 'react-router'
import { Badge, Button, Card, EmptyState, ErrorState, Meter, Mono, PageHeader, SkeletonRows, Td, Th } from '../components/ui'
import { get, list } from '../lib/api'
import { environments, formatCount, formatDateTime, humanAction } from '../lib/format'
import { useCan } from '../lib/session'
import type { AuditEntry, Subnet, Summary } from '../lib/types'

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

export function Dashboard() {
  const canAudit = useCan('audit:view')
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['summary'], queryFn: () => get<Summary>('/dashboard/summary') })
  return (
    <>
      <PageHeader title="Dashboard" description="Address inventory across all routing domains." />
      {isPending && <Card><SkeletonRows /></Card>}
      {error && <Card><ErrorState error={error} onRetry={refetch} /></Card>}
      {data && (
        <div className="space-y-3">
          <div className="grid grid-cols-2 divide-x divide-y divide-line overflow-hidden rounded-md border border-line bg-surface md:grid-cols-3 xl:grid-cols-6 xl:divide-y-0">
            <Stat label="IPv4 subnets" value={data.subnetsV4} to="/subnets?family=4" />
            <Stat label="IPv6 subnets" value={data.subnetsV6} to="/subnets?family=6" />
            <Stat label="Tracked IP records" value={data.ipRecords} />
            <Stat label="Observed active" value={data.observedActive} note="Reachability, not allocation" />
            <Stat label="Open findings" value={data.openFindings} to="/reconciliation" alert />
            <Stat label="Devices" value={data.devices} />
          </div>
          <div className="grid grid-cols-1 divide-y divide-line overflow-hidden rounded-md border border-line bg-surface md:grid-cols-3 md:divide-x md:divide-y-0">
            <Stat label="Pending approvals" value={data.pendingApprovals} to="/requests?tab=all" alert />
            <Stat label="Approved, waiting to be fulfilled" value={data.readyToFulfil} to="/requests?tab=all" />
            <Stat label="Reservations ending within 7 days" value={data.reservationsExpiringSoon} to="/reservations" alert />
          </div>
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
