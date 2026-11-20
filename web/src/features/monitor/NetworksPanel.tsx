import { AlertTriangle, Cloud, Server, ShieldCheck, Wifi, type LucideIcon } from 'lucide-react'
import { formatCount } from '../../lib/format'
import type { Segment } from '../../lib/types'
import { attention } from '../network/segmentList'

const kinds: { kind: Segment['kind']; title: string; icon: LucideIcon; tint: string }[] = [
  { kind: 'dhcp', title: 'DHCP networks', icon: Wifi, tint: 'bg-accent-soft text-accent' },
  { kind: 'server', title: 'Server networks', icon: Server, tint: 'bg-ok-soft text-ok' },
  { kind: 'cloud', title: 'Cloud networks', icon: Cloud, tint: 'bg-surface-2 text-ink-2' },
  { kind: 'vpn', title: 'VPN client pools', icon: ShieldCheck, tint: 'bg-surface-2 text-ink-2' },
]

/** Every network by type, with what is online, registered and usable, as on the dashboard. */
export function NetworksByKind({ segments }: { segments: Segment[] }) {
  return (
    <div className="grid grid-cols-2 gap-2.5">
      {kinds.map(({ kind, title, icon: Icon, tint }) => {
        const rows = segments.filter((s) => s.kind === kind)
        const sum = (f: (s: Segment) => number) => rows.reduce((n, s) => n + f(s), 0)
        const needs = rows.filter((s) => attention(s)).length
        return (
          <div key={kind} className="rounded-lg border border-line p-3">
            <div className="flex items-center gap-2">
              <span aria-hidden className={`grid size-7 place-items-center rounded-md ${tint}`}><Icon size={14} /></span>
              <span className="min-w-0 flex-1 truncate text-[12px] text-ink-3">{title}</span>
              <span className="text-[18px] font-semibold text-ink tabular-nums">{formatCount(rows.length)}</span>
            </div>
            <dl className="mt-2 grid grid-cols-3 gap-1 text-center">
              {([['Online', sum((s) => s.online)], ['Registered', sum((s) => s.registered)], ['Usable', sum((s) => s.usableSize)]] as const).map(([k, v]) => (
                <div key={k} className="rounded-md bg-surface-2 py-1"><dt className="text-[10.5px] text-ink-3">{k}</dt><dd className="text-[13px] font-semibold text-ink tabular-nums">{formatCount(v)}</dd></div>
              ))}
            </dl>
            {needs > 0 && <p className="mt-1.5 flex items-center gap-1 text-[11.5px] text-warn"><AlertTriangle size={11} aria-hidden />{needs} need attention</p>}
          </div>
        )
      })}
    </div>
  )
}

/** The networks with the most going on right now: alerts first, then devices online. */
export function BusiestNetworks({ segments }: { segments: Segment[] }) {
  const rows = segments.filter((s) => s.online > 0 || s.openAlerts > 0).sort((a, b) => b.openAlerts - a.openAlerts || b.online - a.online).slice(0, 8)
  if (rows.length === 0) return <p className="py-4 text-[13px] text-ink-3">Nothing online in the last two scans.</p>
  const max = Math.max(...rows.map((s) => s.online), 1)
  return (
    <ul className="space-y-2">
      {rows.map((s) => (
        <li key={s.id} className="text-[13px]">
          <div className="mb-0.5 flex items-center gap-2">
            <span className="min-w-0 flex-1 truncate text-ink">{s.name}</span>
            {s.openAlerts > 0 && <span className="flex items-center gap-1 text-[11.5px] text-warn"><AlertTriangle size={11} aria-hidden />{s.openAlerts}</span>}
            <span className="text-ink-2 tabular-nums">{formatCount(s.online)} online</span>
          </div>
          <div className="h-1.5 rounded-full bg-track" title={`${s.online} online of ${s.usableSize} usable`}><div className="h-1.5 rounded-full bg-accent" style={{ width: `${Math.max((s.online / max) * 100, 2)}%` }} /></div>
        </li>
      ))}
    </ul>
  )
}
