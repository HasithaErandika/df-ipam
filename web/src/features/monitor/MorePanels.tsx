import { formatCount } from '../../lib/format'
import type { FirewallLive, LinkDashboard, LinkSettings } from '../../lib/types'
import { formatMoney, linkStatuses, typeName } from '../links/labels'

const statusColor: Record<string, string> = { active: 'var(--ok)', pending: 'var(--meter-warning)', inactive: 'var(--line-strong)', terminated: 'var(--danger)' }

/** Network links: how many are in service, the tunnels the firewall sees, types and cost. */
export function LinksSummary({ l, fw, settings }: { l: LinkDashboard; fw?: FirewallLive; settings?: LinkSettings }) {
  const tunnelsUp = fw ? fw.tunnels.filter((t) => t.phaseUp > 0).length : null
  const types = [...l.byType].sort((a, b) => b.count - a.count).slice(0, 4)
  const max = Math.max(...types.map((t) => t.count), 1)
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-3 gap-2 text-center">
        {[['In service', `${l.totals.active}/${l.totals.total}`], ['VPN tunnels', formatCount(l.totals.vpn)], ['Tunnels up', tunnelsUp == null ? '—' : `${tunnelsUp}/${fw!.tunnels.length}`]].map(([k, v]) => (
          <div key={k} className="rounded-lg bg-surface-2 py-2"><div className="text-[11px] text-ink-3">{k}</div><div className="text-[17px] font-semibold text-ink tabular-nums">{v}</div></div>
        ))}
      </div>
      <div>
        <div className="flex h-2.5 gap-[2px] overflow-hidden rounded-full bg-track" role="img" aria-label={l.byStatus.map((s) => `${linkStatuses[s.key]} ${s.count}`).join(', ')}>
          {l.byStatus.map((s) => <div key={s.key} title={`${linkStatuses[s.key]}: ${s.count}`} style={{ flex: s.count, backgroundColor: statusColor[s.key] }} />)}
        </div>
        <div className="mt-1.5 flex flex-wrap gap-x-3 gap-y-0.5 text-[11.5px] text-ink-2">
          {l.byStatus.map((s) => <span key={s.key} className="flex items-center gap-1"><span className="size-2 rounded-[2px]" style={{ backgroundColor: statusColor[s.key] }} />{linkStatuses[s.key]} {s.count}</span>)}
        </div>
      </div>
      <ul className="space-y-1.5">
        {types.map((t) => (
          <li key={t.key} className="text-[12.5px]">
            <div className="flex justify-between gap-2"><span className="truncate text-ink">{typeName(settings, t.key)}</span><span className="text-ink-2 tabular-nums">{t.count}</span></div>
            <div className="h-1.5 rounded-full bg-track"><div className="h-1.5 rounded-full bg-accent" style={{ width: `${(t.count / max) * 100}%` }} /></div>
          </li>
        ))}
      </ul>
      {!l.commercialHidden && (l.monthlyByCurrency ?? []).length > 0 && (
        <p className="border-t border-line pt-2 text-[12.5px] text-ink-2">Monthly cost: <span className="font-medium text-ink">{l.monthlyByCurrency!.map((m) => formatMoney(m.amount, m.currency)).join(' + ')}</span></p>
      )}
    </div>
  )
}
