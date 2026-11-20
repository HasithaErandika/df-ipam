import { useQuery } from '@tanstack/react-query'
import { Plus, Upload } from 'lucide-react'
import { useState } from 'react'
import { Link, useNavigate } from 'react-router'
import { Button, Card, DataTable, EmptyState, ErrorState, PageHeader, SkeletonRows, Stat, StateBadge } from '../../components/ui'
import { get } from '../../lib/api'
import { formatAgo } from '../../lib/format'
import { useCan } from '../../lib/session'
import type { LinkAmount, LinkCount, LinkDashboard } from '../../lib/types'
import { formatMoney, linkStatuses, linkStatusTone, typeName } from './labels'
import { LinkFormModal } from './LinkFormModal'
import { useLinkSettings } from './queries'
import { renewalColumns } from './renewalColumns'

/** Horizontal bars, each a share of the largest. */
function Bars({ rows, to }: { rows: { key: string; label: string; value: number; text: string }[]; to?: (key: string) => string }) {
  if (rows.length === 0) return <p className="text-[13px] text-ink-3">Nothing yet.</p>
  const max = Math.max(...rows.map((r) => r.value), 1)
  return (
    <ul className="space-y-2">
      {rows.map((r) => (
        <li key={r.key + r.text}>
          <div className="mb-0.5 flex justify-between gap-3 text-[12.5px]">
            {to ? <Link to={to(r.key)} className="truncate text-ink hover:text-accent hover:underline">{r.label}</Link> : <span className="truncate text-ink">{r.label}</span>}
            <span className="shrink-0 text-ink-2 tabular-nums">{r.text}</span>
          </div>
          <div className="h-2 rounded-full bg-track"><div className="h-2 rounded-full bg-accent" style={{ width: `${Math.max((r.value / max) * 100, 2)}%` }} /></div>
        </li>
      ))}
    </ul>
  )
}

const counts = (rows: LinkCount[], label: (r: LinkCount) => string = (r) => r.label) =>
  rows.map((r) => ({ key: r.key, label: label(r), value: r.count, text: String(r.count) }))

/** Monthly cost per currency, never added across currencies unless a rate is set. */
function Money({ d }: { d: LinkDashboard }) {
  const totals = d.monthlyByCurrency ?? []
  if (totals.length === 0) return <>None recorded</>
  return <span className="flex flex-col gap-0.5">{totals.map((t) => <span key={t.currency}>{formatMoney(t.amount, t.currency)}</span>)}</span>
}

/** Network links at a glance: counts, costs, renewals and recent changes. */
export function LinksOverviewPage() {
  const navigate = useNavigate()
  const canCreate = useCan('links:create')
  const canImport = useCan('links:import_export')
  const [adding, setAdding] = useState(false)
  const settings = useLinkSettings()
  const q = useQuery({ queryKey: ['/network-links', 'dashboard'], queryFn: () => get<LinkDashboard>('/network-links/dashboard'), refetchInterval: 60_000 })
  const header = <><LinkFormModal open={adding} onClose={() => setAdding(false)} onSaved={(id) => navigate(`/network-links/${id}`)} /><PageHeader title="Network links" description="Administrative records of the company's connectivity. Status says whether a link is in service, not whether it is up right now: monitoring tools show that."
    actions={<>
      {canImport && <Button onClick={() => navigate('/network-links/import-export')}><Upload size={14} aria-hidden />Import</Button>}
      {canCreate && <Button variant="primary" onClick={() => setAdding(true)}><Plus size={14} aria-hidden />Add network link</Button>}
    </>} /></>
  if (q.isPending) return <>{header}<Card><SkeletonRows rows={8} /></Card></>
  if (q.isError) return <>{header}<Card><ErrorState error={q.error} onRetry={() => void q.refetch()} /></Card></>
  const d = q.data
  if (d.totals.total === 0) {
    return <>{header}<Card><EmptyState title="No network links yet" body="Add your internet lines, VPN tunnels and leased lines, or import them from Excel in Import / Export." /></Card></>
  }
  const money = !d.commercialHidden
  const costRows = (d.monthlyByProvider ?? []).map((r: LinkAmount) => ({ key: r.key, label: r.label, value: r.amount, text: formatMoney(r.amount, r.currency) }))
  return (
    <>
      {header}
      <div className="mb-4 grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
        <Stat label="Total links" value={d.totals.total} onClick={() => navigate('/network-links/all')} />
        <Stat label="Active" value={d.totals.active} tone="ok" onClick={() => navigate('/network-links/all?status=active')} />
        <Stat label="Inactive or terminated" value={d.totals.inactive} hint={d.totals.pending ? `${d.totals.pending} pending` : undefined} />
        <Stat label="VPN tunnels" value={d.totals.vpn} onClick={() => navigate('/network-links/all?vpn=true')} />
        {money && <Stat label="Monthly recurring cost" value={<span className="text-[15px]"><Money d={d} /></span>}
          hint={d.monthlyCombinedLkr != null ? `About ${formatMoney(d.monthlyCombinedLkr)} at the set USD rate` : 'Terminated links and links with no separate cost are left out.'} />}
        {money && <Stat label="Contracts ending in 90 days" value={d.totals.expiring} tone={d.totals.expiring ? 'warn' : undefined} onClick={() => navigate('/network-links/renewals')} />}
      </div>
      <div className="grid gap-4 lg:grid-cols-2 xl:grid-cols-3">
        <Card title="Links by type"><div className="p-4"><Bars rows={counts(d.byType, (r) => typeName(settings.data, r.key))} to={(k) => `/network-links/all?linkType=${k}`} /></div></Card>
        <Card title="Links by service provider"><div className="p-4"><Bars rows={counts(d.byProvider)} to={(k) => (k ? `/network-links/all?providerId=${k}` : '/network-links/all')} /></div></Card>
        <Card title="Links by location"><div className="p-4"><Bars rows={counts(d.bySite)} to={(k) => `/network-links/all?site=${encodeURIComponent(k)}`} /></div></Card>
        {money && <Card title="Monthly cost by provider" subtitle="Each currency on its own."><div className="p-4"><Bars rows={costRows} /></div></Card>}
        <Card title="Recently updated" className={money ? undefined : 'xl:col-span-2'}>
          <ul className="divide-y divide-line">
            {d.recent.map((r) => (
              <li key={r.id} className="flex items-center justify-between gap-2 px-4 py-2 text-[13px]">
                <Link to={`/network-links/${r.id}`} className="min-w-0 truncate hover:text-accent hover:underline"><span className="font-mono text-ink-3">{r.code}</span> {r.name}</Link>
                <span className="flex shrink-0 items-center gap-2"><StateBadge tone={linkStatusTone[r.status]}>{linkStatuses[r.status]}</StateBadge><span className="text-[12px] text-ink-3">{formatAgo(r.updatedAt)}</span></span>
              </li>
            ))}
          </ul>
        </Card>
        {money && (
          <div className="lg:col-span-2 xl:col-span-3">
            <Card title="Upcoming contract renewals" subtitle="Ending within 90 days, or already ended." action={<Link to="/network-links/renewals" className="text-[12.5px] text-accent hover:underline">All renewals</Link>}>
              <DataTable columns={renewalColumns} rows={d.renewals ?? []} query={q} rowKey={(r) => r.id} empty={{ title: 'No contracts end in the next 90 days' }} />
            </Card>
          </div>
        )}
      </div>
    </>
  )
}
