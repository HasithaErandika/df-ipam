import { AlertTriangle, CheckCircle2 } from 'lucide-react'
import { Link } from 'react-router'
import type { Dashboard } from '../../lib/types'

/** One line that says whether anything needs someone, with a way to each part. */
export function HealthBanner({ d }: { d: Dashboard }) {
  const urgent = d.alerts.filter((a) => a.severity === 'critical' || a.severity === 'high').reduce((n, a) => n + a.count, 0)
  const down = d.servers.reduce((n, s) => n + s.down, 0)
  const parts = [
    urgent ? { n: urgent, text: `urgent alert${urgent === 1 ? '' : 's'}`, to: '/alerts' } : null,
    down ? { n: down, text: `server${down === 1 ? '' : 's'} down`, to: '/servers?health=down' } : null,
    d.totals.pendingDevices ? { n: d.totals.pendingDevices, text: `device${d.totals.pendingDevices === 1 ? '' : 's'} to review`, to: '/devices' } : null,
  ].filter((p): p is { n: number; text: string; to: string } => p !== null)
  const bad = urgent + down > 0
  if (parts.length === 0) {
    return <div className="mb-4 flex items-center gap-3 rounded-lg border border-ok/25 bg-ok-soft px-4 py-3 text-[14px] text-ok" role="status"><CheckCircle2 size={18} aria-hidden /><span className="font-medium">All clear.</span><span className="text-ink-2">No urgent alerts, no servers down, nothing waiting for review.</span></div>
  }
  return (
    <div className={`mb-4 flex flex-wrap items-center gap-x-4 gap-y-1 rounded-lg border px-4 py-3 text-[14px] ${bad ? 'border-danger/25 bg-danger-soft' : 'border-warn/25 bg-warn-soft'}`} role="status">
      <span className={`flex items-center gap-2 font-medium ${bad ? 'text-danger' : 'text-warn'}`}><AlertTriangle size={18} aria-hidden />Needs attention</span>
      {parts.map((p) => <Link key={p.to} to={p.to} className="text-ink hover:underline"><strong className="tabular-nums">{p.n}</strong> {p.text}</Link>)}
    </div>
  )
}
