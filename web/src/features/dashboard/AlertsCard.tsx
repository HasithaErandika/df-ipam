import { CheckCircle2 } from 'lucide-react'
import { Link } from 'react-router'
import { StackedBar } from '../../components/charts'
import { Card, SeverityBadge } from '../../components/ui'
import { formatAgo } from '../../lib/format'
import type { Dashboard } from '../../lib/types'

const colors = { critical: 'var(--danger)', high: 'var(--meter-warning)', medium: 'var(--accent)', low: 'var(--ink-3)' }

/** Open alerts by severity, and the most pressing ones. */
export function AlertsCard({ d }: { d: Dashboard }) {
  const by = Object.fromEntries(d.alerts.map((a) => [a.severity, a.count])) as Record<string, number>
  const open = d.alerts.reduce((n, a) => n + a.count, 0)
  return (
    <Card title="Alerts" subtitle="Open until their cause is gone"
      action={<Link className="text-[12.5px] font-medium text-accent hover:underline" to="/alerts">All alerts</Link>}>
      {open === 0 ? (
        <div className="flex items-center gap-3 p-5 text-[13px] text-ink-2"><CheckCircle2 size={20} className="text-ok" aria-hidden />No open alerts. Nothing needs attention.</div>
      ) : (
        <div className="space-y-4 p-4">
          <StackedBar label="Open alerts by severity" parts={(['critical', 'high', 'medium', 'low'] as const).map((s) => ({
            key: s, label: s.charAt(0).toUpperCase() + s.slice(1), value: by[s] ?? 0, color: colors[s],
          }))} />
          <ul className="divide-y divide-line rounded-md border border-line">
            {d.recentAlerts.map((a) => (
              <li key={a.id}>
                <Link to={`/alerts?q=${encodeURIComponent(a.ip ?? a.message)}`} className="flex items-start gap-3 px-3 py-2 hover:bg-surface-2">
                  <SeverityBadge severity={a.severity} />
                  <span className="min-w-0 flex-1">
                    <span className="line-clamp-2 text-[13px] leading-snug text-ink">{a.message}</span>
                    <span className="text-[11.5px] text-ink-3">{[a.segment, formatAgo(a.openedAt)].filter(Boolean).join(' · ')}</span>
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </div>
      )}
    </Card>
  )
}
