import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router'
import { StackedBar } from '../../components/charts'
import { Card, EmptyState, ErrorState, PageHeader, SkeletonRows, Stat } from '../../components/ui'
import { get } from '../../lib/api'
import { formatCount } from '../../lib/format'
import { useCan } from '../../lib/session'
import type { Dashboard } from '../../lib/types'
import { ScanSummary } from '../discovery/ScanSummary'
import { FloorCard } from './FloorCard'

const severityColors = { critical: 'var(--danger)', high: 'var(--meter-warning)', medium: 'var(--accent)', low: 'var(--ink-3)' }

export function DashboardPage() {
  const navigate = useNavigate()
  const canApprove = useCan('requests:approve')
  const canAsk = useCan('ip:request')
  const canRequests = canApprove || canAsk
  const query = useQuery({ queryKey: ['/dashboard'], queryFn: () => get<Dashboard>('/dashboard'), refetchInterval: 60_000 })
  if (query.isPending) return <><PageHeader title="Dashboard" /><SkeletonRows rows={8} /></>
  if (query.isError) return <><PageHeader title="Dashboard" /><Card><ErrorState error={query.error} onRetry={() => void query.refetch()} /></Card></>
  const d = query.data
  const alerts = Object.fromEntries(d.alerts.map((a) => [a.severity, a.count])) as Record<string, number>
  const urgent = (alerts.critical ?? 0) + (alerts.high ?? 0)
  const open = d.alerts.reduce((n, a) => n + a.count, 0)
  const down = d.servers.reduce((n, s) => n + s.down, 0)
  const monitored = d.servers.reduce((n, s) => n + s.total, 0)

  return (
    <>
      <PageHeader title="Dashboard" description="The network at a glance. Every figure links to the list behind it." actions={<ScanSummary compact />} />
      <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
        <Stat label="Open alerts" value={formatCount(open)} tone={urgent ? 'danger' : undefined} hint={urgent ? `${urgent} High or Critical` : 'None urgent'} onClick={() => navigate('/alerts')} />
        <Stat label="Servers down" value={formatCount(down)} tone={down ? 'danger' : monitored ? 'ok' : undefined} hint={`${formatCount(monitored)} monitored`} onClick={() => navigate('/servers?health=down')} />
        <Stat label="Devices to review" value={formatCount(d.totals.pendingDevices)} tone={d.totals.pendingDevices ? 'warn' : undefined} hint="Unknown MACs seen" onClick={() => navigate('/devices')} />
        <Stat label="Online now" value={formatCount(d.totals.online)} hint="Addresses answering" onClick={() => navigate('/discovery?kind=online')} />
        {canRequests && <Stat label="Pending requests" value={formatCount(d.totals.pendingRequests)} tone={d.totals.pendingRequests ? 'warn' : undefined} hint="Waiting for a decision" onClick={() => navigate('/requests')} />}
        <Stat label="Expiring holds" value={formatCount(d.totals.expiringReservations)} tone={d.totals.expiringReservations ? 'warn' : undefined} hint="Reservations, next 7 days" onClick={() => navigate('/reservations/expiring')} />
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <Card title="IP register" subtitle={`${formatCount(d.totals.registered)} registered addresses, plus reservations`}>
          <div className="p-4">
            <StackedBar label="IP register by status" parts={[
              { key: 'assigned', label: 'Assigned', value: d.totals.assigned, color: 'var(--addr-assigned)' },
              { key: 'reserved', label: 'Reserved', value: d.totals.reserved, color: 'var(--addr-reserved)' },
              { key: 'held', label: 'Held by reservations', value: d.totals.held, color: 'var(--addr-held)' },
              { key: 'blocked', label: 'Blocked', value: d.totals.blocked, color: 'var(--addr-blocked)' },
            ]} />
          </div>
        </Card>
        <Card title="Open alerts by severity" subtitle="Alerts close by themselves when the cause is gone">
          <div className="p-4">
            {open === 0 ? <p className="py-3 text-[13px] text-ink-3">No open alerts. Nothing needs attention.</p> : (
              <StackedBar label="Open alerts by severity" parts={(['critical', 'high', 'medium', 'low'] as const).map((s) => ({
                key: s, label: s.charAt(0).toUpperCase() + s.slice(1), value: alerts[s] ?? 0, color: severityColors[s],
              }))} />
            )}
          </div>
        </Card>
      </div>

      <Card title="Floors" subtitle="DHCP pool use and addresses seen in the last two scans" className="mt-4">
        {d.floors.length === 0
          ? <EmptyState title="No floor segments yet" body="Add each floor's DHCP segment in Settings > Segments to see its usage here." />
          : <div className="grid gap-3 p-4 sm:grid-cols-2 xl:grid-cols-4">{d.floors.map((f) => <FloorCard key={f.id} floor={f} />)}</div>}
      </Card>

      <Card title="Servers by environment" subtitle="Monitored entries and their latest check" className="mt-4">
        {d.servers.length === 0 ? <EmptyState title="No monitored servers" body="Turn on Monitor for a register entry to check it." /> : (
          <ul className="divide-y divide-line">
            {d.servers.map((s) => (
              <li key={s.environment} className="grid items-center gap-3 px-4 py-3 sm:grid-cols-[180px_1fr]">
                <div className="flex items-center gap-2 text-[13px] font-medium text-ink">
                  <span aria-hidden className="size-2.5 rounded-[2px]" style={{ backgroundColor: s.colour }} />{s.environment}
                  <span className="font-normal text-ink-3">· {s.total}</span>
                </div>
                <StackedBar label={`${s.environment} server health`} height={10} parts={[
                  { key: 'up', label: 'Up', value: s.up, color: 'var(--ok)' },
                  { key: 'down', label: 'Down', value: s.down, color: 'var(--danger)' },
                  { key: 'unknown', label: 'Not checked yet', value: s.unknown, color: 'var(--line-strong)' },
                ]} />
              </li>
            ))}
          </ul>
        )}
      </Card>
    </>
  )
}
