import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'
import { Card } from '../../components/ui'
import { get } from '../../lib/api'
import { useCan } from '../../lib/session'
import type { LinkDashboard } from '../../lib/types'
import { formatMoney } from '../links/labels'

/** Network links in brief, for people who may see them. */
export function LinksCard() {
  const allowed = useCan('links:view')
  const q = useQuery({ queryKey: ['/network-links', 'dashboard'], queryFn: () => get<LinkDashboard>('/network-links/dashboard'), enabled: allowed, refetchInterval: 60_000 })
  if (!allowed || !q.data) return null
  const d = q.data
  const row = (label: string, value: React.ReactNode, to?: string) => (
    <li className="flex items-center justify-between gap-3 py-1.5 text-[13px]">
      <span className="text-ink-2">{label}</span>{to ? <Link to={to} className="font-medium text-ink tabular-nums hover:text-accent hover:underline">{value}</Link> : <span className="font-medium text-ink tabular-nums">{value}</span>}
    </li>
  )
  return (
    <Card title="Network links" subtitle="Internet lines, VPN tunnels and leased lines" action={<Link to="/network-links" className="text-[12.5px] text-accent hover:underline">Overview</Link>}>
      <ul className="divide-y divide-line px-4 py-1">
        {row('Active links', `${d.totals.active} of ${d.totals.total}`, '/network-links/all?status=active')}
        {row('VPN tunnels', d.totals.vpn, '/network-links/all?vpn=true')}
        {!d.commercialHidden && row('Monthly cost', (d.monthlyByCurrency ?? []).map((m) => formatMoney(m.amount, m.currency)).join(' + ') || '—')}
        {!d.commercialHidden && row('Contracts ending in 90 days', <span className={d.totals.expiring ? 'text-warn' : undefined}>{d.totals.expiring}</span>, '/network-links/renewals')}
      </ul>
    </Card>
  )
}
