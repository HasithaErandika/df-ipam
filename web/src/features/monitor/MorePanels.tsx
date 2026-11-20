import { CalendarClock, ClipboardList, Hourglass, MonitorSmartphone, ShieldQuestion, Timer, type LucideIcon } from 'lucide-react'
import { formatCount } from '../../lib/format'
import type { Dashboard, FirewallLive, LinkDashboard, LinkSettings } from '../../lib/types'
import { formatMoney, linkStatuses, typeName } from '../links/labels'
import { AllClear } from './MonitorPanels'

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

interface Waiting { icon: LucideIcon; label: string; n: number; detail: string; tone: 'warn' | 'danger' | 'neutral' }

/** Everything waiting for a person: decisions, requests, expiring holds and contracts. */
export function WaitingFor({ d, l }: { d: Dashboard; l?: LinkDashboard }) {
  const soon = l?.renewals?.filter((r) => r.daysLeft <= 30).length ?? 0
  const items: Waiting[] = [
    { icon: MonitorSmartphone, label: 'Devices to approve or reject', n: d.totals.pendingDevices, detail: 'New MACs seen on the network', tone: 'warn' },
    { icon: ShieldQuestion, label: 'Unknown devices', n: d.totals.unknownDevices, detail: 'Seen, never approved', tone: 'warn' },
    { icon: ClipboardList, label: 'IP requests to decide', n: d.totals.pendingRequests, detail: 'Waiting for an approver', tone: 'warn' },
    { icon: Timer, label: 'Reservations expiring', n: d.totals.expiringReservations, detail: 'Within 7 days', tone: 'neutral' },
    ...(l ? [{ icon: Hourglass, label: 'Links pending', n: l.totals.pending, detail: 'Not yet in service', tone: 'neutral' } as Waiting] : []),
    ...(l && !l.commercialHidden ? [{ icon: CalendarClock, label: 'Contracts to renew', n: l.totals.expiring, detail: soon ? `${soon} within 30 days` : 'Within 90 days', tone: soon ? 'danger' : 'warn' } as Waiting] : []),
  ]
  const open = items.filter((i) => i.n > 0)
  if (open.length === 0) return <AllClear text="Nothing is waiting for anyone" />
  const tint = { warn: 'bg-warn-soft text-warn', danger: 'bg-danger-soft text-danger', neutral: 'bg-surface-2 text-ink-2' }
  return (
    <ul className="space-y-1.5">
      {open.map(({ icon: Icon, label, n, detail, tone }) => (
        <li key={label} className="flex items-center gap-3 rounded-lg px-1 py-1">
          <span aria-hidden className={`grid size-8 shrink-0 place-items-center rounded-lg ${tint[tone]}`}><Icon size={15} /></span>
          <span className="min-w-0 flex-1"><span className="block truncate text-[13px] text-ink">{label}</span><span className="block truncate text-[11.5px] text-ink-3">{detail}</span></span>
          <span className="text-[18px] font-semibold text-ink tabular-nums">{formatCount(n)}</span>
        </li>
      ))}
    </ul>
  )
}
