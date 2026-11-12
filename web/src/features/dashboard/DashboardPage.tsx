import { useQuery } from '@tanstack/react-query'
import { useNavigate } from 'react-router'
import { StackedBar } from '../../components/charts'
import { Card, EmptyState, ErrorState, PageHeader, SkeletonRows, Stat } from '../../components/ui'
import { get } from '../../lib/api'
import { formatCount } from '../../lib/format'
import { useCan, useMe } from '../../lib/session'
import type { Dashboard } from '../../lib/types'
import { ScanSummary } from '../discovery/ScanSummary'
import { AlertsCard } from './AlertsCard'
import { DhcpNetworkCard } from './DhcpNetworkCard'
import { NetworksCard } from './NetworksCard'

function greeting() {
  const h = new Date().getHours()
  return h < 12 ? 'Good morning' : h < 17 ? 'Good afternoon' : 'Good evening'
}

export function DashboardPage() {
  const navigate = useNavigate()
  const me = useMe()
  const canApprove = useCan('requests:approve')
  const canAsk = useCan('ip:request')
  const canRequests = canApprove || canAsk
  const query = useQuery({ queryKey: ['/dashboard'], queryFn: () => get<Dashboard>('/dashboard'), refetchInterval: 60_000 })
  const header = <PageHeader title="Dashboard" description={`${greeting()}. The network at a glance; every figure opens the list behind it.`} actions={<ScanSummary compact />} />
  if (query.isPending) return <>{header}<SkeletonRows rows={8} /></>
  if (query.isError) return <>{header}<Card><ErrorState error={query.error} onRetry={() => void query.refetch()} /></Card></>
  const d = query.data
  const t = d.totals
  const urgent = d.alerts.filter((a) => a.severity === 'critical' || a.severity === 'high').reduce((n, a) => n + a.count, 0)
  const open = d.alerts.reduce((n, a) => n + a.count, 0)
  const down = d.servers.reduce((n, s) => n + s.down, 0)
  const monitored = d.servers.reduce((n, s) => n + s.total, 0)

  return (
    <>
      {header}
      <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
        <Stat label="Open alerts" value={formatCount(open)} tone={urgent ? 'danger' : open ? 'warn' : 'ok'} hint={urgent ? `${urgent} High or Critical` : open ? 'None urgent' : 'All clear'} onClick={() => navigate('/alerts')} />
        <Stat label="Online now" value={formatCount(t.online)} hint={`${formatCount(t.newLastHour)} new in the last hour`} onClick={() => navigate('/discovery')} />
        <Stat label="Devices to review" value={formatCount(t.pendingDevices)} tone={t.pendingDevices ? 'warn' : undefined} hint={`${formatCount(t.approvedDevices)} approved`} onClick={() => navigate('/devices')} />
        <Stat label="Servers down" value={formatCount(down)} tone={down ? 'danger' : monitored ? 'ok' : undefined} hint={monitored ? `${formatCount(monitored)} monitored` : 'None monitored yet'} onClick={() => navigate('/servers?health=down')} />
        <Stat label="IP register" value={formatCount(t.registered)} hint={`${formatCount(t.assigned)} assigned`} onClick={() => navigate('/servers/register')} />
        {canRequests
          ? <Stat label="Pending requests" value={formatCount(t.pendingRequests)} tone={t.pendingRequests ? 'warn' : undefined} hint={`${formatCount(t.expiringReservations)} holds expiring in 7 days`} onClick={() => navigate('/requests')} />
          : <Stat label="Expiring holds" value={formatCount(t.expiringReservations)} tone={t.expiringReservations ? 'warn' : undefined} hint="Reservations, next 7 days" onClick={() => navigate('/reservations/expiring')} />}
      </div>

      <div className="mt-4 grid gap-4 xl:grid-cols-[3fr_2fr]">
        <NetworksCard />
        <AlertsCard d={d} />
      </div>

      <Card title="DHCP networks" subtitle="Floors and Wi-Fi: pool use, and what answered in the last two scans" className="mt-4">
        {d.floors.length === 0
          ? <EmptyState title="No DHCP networks yet" body="Add each floor or Wi-Fi network, with its DHCP pool, in Network setup > Segments to see its use here." />
          : <div className="grid gap-3 p-4 sm:grid-cols-2 xl:grid-cols-4">{d.floors.map((f) => <DhcpNetworkCard key={f.id} network={f} />)}</div>}
      </Card>

      <div className="mt-4 grid gap-4 lg:grid-cols-3">
        <Card title="Servers by environment" subtitle="Monitored entries and their latest check">
          {d.servers.length === 0 ? <EmptyState title="No monitored servers" body="Turn on Monitor for a server in the register to check it." /> : (
            <ul className="divide-y divide-line">
              {d.servers.map((s) => (
                <li key={s.environment} className="px-4 py-3">
                  <div className="mb-1.5 flex items-center gap-2 text-[13px] font-medium text-ink">
                    <span aria-hidden className="size-2.5 rounded-[2px]" style={{ backgroundColor: s.colour }} />{s.environment}
                    <span className="ml-auto font-normal text-ink-3 tabular-nums">{s.down ? <span className="text-danger">{s.down} down</span> : `${s.up} up`} · {s.total}</span>
                  </div>
                  <StackedBar label={`${s.environment} server health`} height={8} parts={[
                    { key: 'up', label: 'Up', value: s.up, color: 'var(--ok)' },
                    { key: 'down', label: 'Down', value: s.down, color: 'var(--danger)' },
                    { key: 'unknown', label: 'Not checked yet', value: s.unknown, color: 'var(--line-strong)' },
                  ]} />
                </li>
              ))}
            </ul>
          )}
        </Card>
        <Card title="IP register" subtitle={`${formatCount(t.registered)} addresses, plus ${formatCount(t.held)} held by reservations`}>
          <div className="p-4">
            {t.registered + t.held === 0 ? <p className="py-3 text-[13px] text-ink-3">Nothing registered yet. Register addresses from Discovery or the Servers page.</p> : (
              <StackedBar label="IP register by status" parts={[
                { key: 'assigned', label: 'Assigned', value: t.assigned, color: 'var(--addr-assigned)' },
                { key: 'reserved', label: 'Reserved', value: t.reserved, color: 'var(--addr-reserved)' },
                { key: 'held', label: 'Held by reservations', value: t.held, color: 'var(--addr-held)' },
                { key: 'blocked', label: 'Blocked', value: t.blocked, color: 'var(--addr-blocked)' },
              ]} />
            )}
          </div>
        </Card>
        <Card title="Devices" subtitle="Known MAC addresses and decisions">
          <div className="p-4">
            {t.approvedDevices + t.pendingDevices + t.rejectedDevices === 0 ? <p className="py-3 text-[13px] text-ink-3">No devices yet. MACs seen by the scanner appear here for a decision.</p> : (
              <StackedBar label="Devices by decision" parts={[
                { key: 'approved', label: 'Approved', value: t.approvedDevices, color: 'var(--ok)' },
                { key: 'pending', label: 'Waiting for a decision', value: t.pendingDevices, color: 'var(--meter-warning)' },
                { key: 'rejected', label: 'Rejected', value: t.rejectedDevices, color: 'var(--danger)' },
              ]} />
            )}
          </div>
        </Card>
      </div>
      <p className="mt-4 text-center text-[11.5px] text-ink-3">Signed in as {me.email}{me.roleName ? ` · ${me.roleName}` : ''}. Figures refresh every minute.</p>
    </>
  )
}
