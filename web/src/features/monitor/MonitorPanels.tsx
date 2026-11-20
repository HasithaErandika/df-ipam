import type { ReactNode } from 'react'
import { type LucideIcon } from 'lucide-react'
import { Donut, LineChart, UseBar } from '../../components/monitorCharts'
import { formatCount } from '../../lib/format'
import { severities } from '../../lib/labels'
import type { Dashboard, FirewallLive } from '../../lib/types'
import { formatRate } from '../firewall/format'
import type { TrafficPoint } from './useMonitorData'

/** A panel on the monitor, with an icon that says what it is about. */
export function Panel({ title, icon: Icon, note, children, className = '' }: { title: string; icon: LucideIcon; note?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={`flex min-h-0 flex-col overflow-hidden rounded-xl border border-line bg-surface p-4 shadow-[0_1px_2px_rgba(11,18,32,.04)] ${className}`}>
      <header className="mb-3 flex shrink-0 items-center gap-2">
        <span aria-hidden className="grid size-7 place-items-center rounded-md bg-accent-soft text-accent"><Icon size={15} /></span>
        <h2 className="text-[14px] font-semibold text-ink">{title}</h2>
        {note && <span className="ml-auto truncate text-[12px] text-ink-3">{note}</span>}
      </header>
      <div className="min-h-0 flex-1 overflow-hidden">{children}</div>
    </section>
  )
}

const tileTone = {
  ok: { box: 'border-line', icon: 'bg-ok-soft text-ok', ink: 'text-ink', word: 'text-ok' },
  warn: { box: 'border-warn/30', icon: 'bg-warn-soft text-warn', ink: 'text-ink', word: 'text-warn' },
  danger: { box: 'border-danger/40 bg-danger-soft/30', icon: 'bg-danger-soft text-danger', ink: 'text-danger', word: 'text-danger' },
  neutral: { box: 'border-line', icon: 'bg-surface-2 text-ink-2', ink: 'text-ink', word: 'text-ink-3' },
}

/** One headline number with an icon, and its state in words as well as colour. */
export function Tile({ label, value, state, hint, icon: Icon }: { label: string; value: ReactNode; state: keyof typeof tileTone; hint?: string; icon: LucideIcon }) {
  const t = tileTone[state]
  return (
    <div className={`flex items-center gap-3 rounded-xl border bg-surface px-4 py-3 ${t.box}`}>
      <span aria-hidden className={`grid size-10 shrink-0 place-items-center rounded-lg ${t.icon}`}><Icon size={19} /></span>
      <div className="min-w-0">
        <div className="truncate text-[12px] font-medium text-ink-2">{label}</div>
        <div className={`text-[24px] leading-tight font-semibold tabular-nums ${t.ink}`}>{value}</div>
        {hint && <div className={`truncate text-[11.5px] ${t.word}`}>{hint}</div>}
      </div>
    </div>
  )
}

const sevColor: Record<string, string> = { critical: 'var(--danger)', high: 'var(--meter-warning)', medium: 'var(--accent)', low: 'var(--line-strong)' }

export function AlertsDonut({ d }: { d: Dashboard }) {
  const parts = ['critical', 'high', 'medium', 'low'].map((s) => ({ key: s, label: severities[s], value: d.alerts.find((a) => a.severity === s)?.count ?? 0, color: sevColor[s] }))
  const total = parts.reduce((n, p) => n + p.value, 0)
  return <Donut size={132} caption="Open alerts by severity" parts={parts} center={<div><div className="text-[26px] leading-none font-semibold text-ink">{total}</div><div className="mt-1 text-[12px] text-ink-3">open</div></div>} />
}

export function ServerHealth({ d }: { d: Dashboard }) {
  if (d.servers.length === 0) return <p className="text-[13px] text-ink-3">No monitored servers.</p>
  return (
    <ul className="space-y-3">
      {d.servers.map((s) => (
        <li key={s.environment}>
          <div className="mb-1 flex items-center gap-2 text-[13px]">
            <span aria-hidden className="size-3 rounded-[3px]" style={{ backgroundColor: s.colour }} />
            <span className="font-medium text-ink">{s.environment}</span>
            <span className="ml-auto tabular-nums">{s.down ? <span className="font-semibold text-danger">{s.down} down</span> : <span className="text-ok">All up</span>}<span className="text-ink-3"> · {s.total}</span></span>
          </div>
          <div className="flex h-3 gap-[2px] overflow-hidden rounded-full bg-track" title={`${s.up} up, ${s.down} down, ${s.unknown} not checked`}>
            {[['var(--ok)', s.up], ['var(--danger)', s.down], ['var(--line-strong)', s.unknown]].map(([c, v]) => Number(v) > 0 && <div key={String(c)} style={{ flex: Number(v), backgroundColor: String(c) }} />)}
          </div>
        </li>
      ))}
    </ul>
  )
}

export function Traffic({ points, live }: { points: TrafficPoint[]; live?: FirewallLive }) {
  return (
    <div className="space-y-3">
      <LineChart caption="WAN traffic, all WAN interfaces" times={points.map((p) => p.at)} format={formatRate} height={200}
        series={[{ key: 'rx', label: 'Download', color: 'var(--accent)', values: points.map((p) => p.rx) }, { key: 'tx', label: 'Upload', color: 'var(--addr-reserved)', values: points.map((p) => p.tx) }]} />
      {live && (
        <ul className="grid gap-2 sm:grid-cols-2">
          {live.interfaces.filter((i) => i.role === 'wan' || i.link).map((i) => (
            <li key={i.name} className="flex items-center justify-between gap-2 rounded-lg bg-surface-2 px-3 py-2 text-[14px]">
              <span className="flex min-w-0 items-center gap-2"><span className={`size-2.5 shrink-0 rounded-full ${i.up ? 'bg-ok' : 'bg-danger'}`} /><span className="truncate">{i.link?.name || i.alias || i.name}</span>{!i.up && <span className="font-semibold text-danger">Down</span>}</span>
              <span className="shrink-0 text-ink-2 tabular-nums">↓ {formatRate(i.rxBps)} · ↑ {formatRate(i.txBps)}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

export function Pools({ d }: { d: Dashboard }) {
  const pools = d.floors.filter((f) => f.poolSize > 0).map((f) => ({ ...f, pct: (f.poolUsed / f.poolSize) * 100 })).sort((a, b) => b.pct - a.pct)
  if (pools.length === 0) return <p className="text-[13px] text-ink-3">No DHCP pools set up.</p>
  return <div className="space-y-3">{pools.slice(0, 6).map((f) => <UseBar key={f.id} label={f.name} pct={f.pct} detail={`${formatCount(f.poolUsed)} of ${formatCount(f.poolSize)} addresses`} />)}</div>
}
