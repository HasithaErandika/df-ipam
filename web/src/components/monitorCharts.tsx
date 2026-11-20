import { useState } from 'react'

/** One slice or series: its name, value(s) and colour token. */
export interface Slice { key: string; label: string; value: number; color: string }

/**
 * A donut for a few parts of one whole, with the total in the middle and a
 * legend that names each part with its count. Hover a part for its share.
 */
export function Donut({ parts, center, caption, size = 168 }: { parts: Slice[]; center: React.ReactNode; caption: string; size?: number }) {
  const [hover, setHover] = useState<string | null>(null)
  const total = parts.reduce((n, p) => n + p.value, 0)
  const r = 15.9155 // a circle of circumference 100
  let offset = 25
  return (
    <div className="flex flex-wrap items-center gap-6">
      <div className="relative" style={{ width: size, height: size }}>
        <svg viewBox="0 0 36 36" width={size} height={size} role="img" aria-label={`${caption}: ${parts.map((p) => `${p.label} ${p.value}`).join(', ')}`}>
          <circle cx="18" cy="18" r={r} fill="none" stroke="var(--track)" strokeWidth="3.2" />
          {total > 0 && parts.filter((p) => p.value > 0).map((p) => {
            const len = (p.value / total) * 100
            const gap = parts.filter((x) => x.value > 0).length > 1 ? 0.8 : 0
            const el = (
              <circle key={p.key} cx="18" cy="18" r={r} fill="none" stroke={p.color} strokeWidth={hover === p.key ? 4.2 : 3.2}
                strokeDasharray={`${Math.max(len - gap, 0.1)} ${100 - Math.max(len - gap, 0.1)}`} strokeDashoffset={offset}
                onMouseEnter={() => setHover(p.key)} onMouseLeave={() => setHover(null)} className="cursor-default transition-[stroke-width]">
                <title>{`${p.label}: ${p.value} (${Math.round(len)}%)`}</title>
              </circle>
            )
            offset -= len
            return el
          })}
        </svg>
        <div className="pointer-events-none absolute inset-0 grid place-items-center text-center">{center}</div>
      </div>
      <ul className="space-y-2">
        {parts.map((p) => (
          <li key={p.key} className={`flex items-center gap-2 text-[13px] ${hover && hover !== p.key ? 'opacity-50' : ''}`} onMouseEnter={() => setHover(p.key)} onMouseLeave={() => setHover(null)}>
            <span aria-hidden className="size-2.5 rounded-[3px]" style={{ backgroundColor: p.color }} />
            <span className="text-ink-2">{p.label}</span>
            <span className="ml-auto pl-4 font-semibold text-ink tabular-nums">{p.value}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}

export interface Series { key: string; label: string; color: string; values: (number | null)[] }

/**
 * Lines over time on one axis, with a recessive grid and a crosshair that
 * shows every series' value at the time under the pointer.
 */
export function LineChart({ series, times, format, height = 180, caption }: {
  series: Series[]; times: number[]; format: (v: number) => string; height?: number; caption: string
}) {
  const [at, setAt] = useState<number | null>(null)
  const w = 600
  const pad = { l: 64, r: 8, t: 8, b: 22 }
  const all = series.flatMap((s) => s.values.filter((v): v is number => v != null))
  const max = Math.max(...all, 1) * 1.15
  const n = times.length
  const x = (i: number) => pad.l + (n <= 1 ? 0 : (i / (n - 1)) * (w - pad.l - pad.r))
  const y = (v: number) => pad.t + (1 - v / max) * (height - pad.t - pad.b)
  const path = (vals: (number | null)[]) => vals.map((v, i) => (v == null ? '' : `${i === 0 || vals[i - 1] == null ? 'M' : 'L'}${x(i).toFixed(1)},${y(v).toFixed(1)}`)).join('')
  const ticks = [0, 0.5, 1].map((f) => f * (max / 1.15))
  const move = (e: React.MouseEvent<SVGSVGElement>) => {
    const box = e.currentTarget.getBoundingClientRect()
    const px = ((e.clientX - box.left) / box.width) * w
    setAt(n <= 1 ? 0 : Math.max(0, Math.min(n - 1, Math.round(((px - pad.l) / (w - pad.l - pad.r)) * (n - 1)))))
  }
  if (n < 2) return <p className="grid place-items-center text-[14px] text-ink-3" style={{ height }}>Collecting samples…</p>
  return (
    <div className="relative">
      <svg viewBox={`0 0 ${w} ${height}`} className="w-full" role="img" aria-label={caption} onMouseMove={move} onMouseLeave={() => setAt(null)}>
        {ticks.map((t) => (
          <g key={t}>
            <line x1={pad.l} x2={w - pad.r} y1={y(t)} y2={y(t)} stroke="var(--line)" strokeWidth="1" />
            <text x={pad.l - 8} y={y(t) + 4} textAnchor="end" fontSize="11" fill="var(--ink-3)">{format(t)}</text>
          </g>
        ))}
        <text x={pad.l} y={height - 4} fontSize="11" fill="var(--ink-3)">{new Date(times[0]).toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' })}</text>
        <text x={w - pad.r} y={height - 4} textAnchor="end" fontSize="11" fill="var(--ink-3)">now</text>
        {series.map((s) => <path key={s.key} d={path(s.values)} fill="none" stroke={s.color} strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" />)}
        {at != null && <>
          <line x1={x(at)} x2={x(at)} y1={pad.t} y2={height - pad.b} stroke="var(--ink-3)" strokeWidth="1" strokeDasharray="3 3" />
          {series.map((s) => s.values[at] != null && <circle key={s.key} cx={x(at)} cy={y(s.values[at]!)} r="4" fill={s.color} stroke="var(--surface)" strokeWidth="2" />)}
        </>}
      </svg>
      {at != null && (
        <div className="pointer-events-none absolute top-1 right-2 rounded-md border border-line bg-surface px-2.5 py-1.5 text-[12.5px] shadow-sm">
          <div className="text-ink-3">{new Date(times[at]).toLocaleTimeString('en-GB')}</div>
          {series.map((s) => <div key={s.key} className="flex items-center gap-2"><span className="size-2 rounded-full" style={{ backgroundColor: s.color }} />{s.label}: <span className="font-medium tabular-nums">{s.values[at] == null ? '—' : format(s.values[at]!)}</span></div>)}
        </div>
      )}
      <div className="mt-1 flex gap-4 text-[12.5px] text-ink-2">
        {series.map((s) => <span key={s.key} className="flex items-center gap-1.5"><span className="h-0.5 w-4 rounded" style={{ backgroundColor: s.color }} />{s.label}</span>)}
      </div>
    </div>
  )
}

/** A share of something, coloured by how close it is to full, with its label. */
export function UseBar({ label, pct, detail }: { label: string; pct: number; detail: string }) {
  const tone = pct >= 90 ? 'var(--danger)' : pct >= 75 ? 'var(--meter-warning)' : 'var(--ok)'
  const word = pct >= 90 ? 'Nearly full' : pct >= 75 ? 'Filling' : ''
  return (
    <div title={`${label}: ${pct.toFixed(0)}% · ${detail}`}>
      <div className="mb-1 flex items-baseline justify-between gap-3 text-[14px]">
        <span className="truncate text-ink">{label}</span>
        <span className="shrink-0 text-ink-2 tabular-nums">{word && <span className={pct >= 90 ? 'text-danger' : 'text-warn'}>{word} · </span>}{pct.toFixed(0)}%</span>
      </div>
      <div className="h-2.5 rounded-full bg-track"><div className="h-2.5 rounded-full" style={{ width: `${Math.max(Math.min(pct, 100), 1.5)}%`, backgroundColor: tone }} /></div>
      <div className="mt-0.5 text-[12px] text-ink-3">{detail}</div>
    </div>
  )
}
