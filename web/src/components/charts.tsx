import { useId, useState } from 'react'
import { EmptyState } from './ui'
import { formatCount } from '../lib/format'

interface Point { label: string; value: number | null }

const width = 480
const height = 150
const pad = { top: 12, right: 12, bottom: 22, left: 40 }

/** A round top for the axis, so the top gridline lands on a readable number. */
function niceMax(max: number): number {
  if (max <= 0) return 1
  const step = 10 ** Math.floor(Math.log10(max))
  return Math.ceil(max / step) * step
}

function shortDate(iso: string): string {
  return new Date(`${iso}T00:00:00Z`).toLocaleDateString('en-GB', { day: 'numeric', month: 'short', timeZone: 'UTC' })
}

/** One series over time. Days with no data break the line instead of reading as zero. */
export function LineChart({ title, points, unit = '', emptyBody }: { title: string; points: Point[]; unit?: string; emptyBody: string }) {
  const [hover, setHover] = useState<number | null>(null)
  const tableId = useId()
  const known = points.filter((p) => p.value !== null) as { label: string; value: number }[]
  if (known.length === 0 || known.every((p) => p.value === 0)) return <EmptyState title="No data yet" body={emptyBody} />
  const top = niceMax(Math.max(...known.map((p) => p.value)))
  const x = (i: number) => pad.left + (i / Math.max(points.length - 1, 1)) * (width - pad.left - pad.right)
  const y = (v: number) => pad.top + (1 - v / top) * (height - pad.top - pad.bottom)
  const runs: string[] = []
  let current = ''
  points.forEach((p, i) => {
    if (p.value === null) {
      if (current) runs.push(current)
      current = ''
      return
    }
    current += `${current ? 'L' : 'M'}${x(i).toFixed(1)} ${y(p.value).toFixed(1)}`
  })
  if (current) runs.push(current)
  const active = hover === null ? null : points[hover]
  const format = (v: number) => `${unit === '%' ? v.toFixed(1) : formatCount(v)}${unit}`
  const move = (clientX: number, box: DOMRect) => {
    const at = ((clientX - box.left) / box.width) * width
    const i = Math.round(((at - pad.left) / (width - pad.left - pad.right)) * (points.length - 1))
    setHover(Math.min(Math.max(i, 0), points.length - 1))
  }
  return (
    <div className="relative p-3">
      <svg viewBox={`0 0 ${width} ${height}`} role="img" aria-label={`${title}. Figures are in the table that follows.`} aria-describedby={tableId} className="w-full touch-none"
        onPointerMove={(e) => move(e.clientX, e.currentTarget.getBoundingClientRect())} onPointerLeave={() => setHover(null)}>
        {[0, top / 2, top].map((v) => (
          <g key={v}>
            <line x1={pad.left} x2={width - pad.right} y1={y(v)} y2={y(v)} stroke="var(--line)" />
            <text x={pad.left - 6} y={y(v) + 3} textAnchor="end" fontSize="10" fill="var(--ink-3)">{format(v)}</text>
          </g>
        ))}
        <text x={pad.left} y={height - 6} fontSize="10" fill="var(--ink-3)">{shortDate(points[0].label)}</text>
        <text x={width - pad.right} y={height - 6} textAnchor="end" fontSize="10" fill="var(--ink-3)">{shortDate(points[points.length - 1].label)}</text>
        {runs.map((d) => <path key={d} d={d} fill="none" stroke="var(--accent)" strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" />)}
        {/* A single reading has no line to draw, so it needs its own mark. */}
        {known.length === 1 && <circle cx={x(points.indexOf(known[0]))} cy={y(known[0].value)} r="4" fill="var(--accent)" />}
        {hover !== null && <line x1={x(hover)} x2={x(hover)} y1={pad.top} y2={height - pad.bottom} stroke="var(--line-strong)" />}
        {hover !== null && active?.value !== null && active && <circle cx={x(hover)} cy={y(active.value as number)} r="4" fill="var(--accent)" stroke="var(--surface)" strokeWidth="2" />}
      </svg>
      <div aria-live="polite" className="h-5 text-right text-xs text-ink-2 tabular-nums">
        {active && `${shortDate(active.label)}: ${active.value === null ? 'No data' : format(active.value)}`}
      </div>
      <table id={tableId} className="sr-only">
        <caption>{title}</caption>
        <thead><tr><th>Date</th><th>Value</th></tr></thead>
        <tbody>{points.map((p) => <tr key={p.label}><td>{p.label}</td><td>{p.value === null ? 'No data' : format(p.value)}</td></tr>)}</tbody>
      </table>
    </div>
  )
}

/** Ranked counts. Every bar carries its own label and value, so colour identifies nothing. */
export function BarList({ title, items }: { title: string; items: { label: string; count: number }[] }) {
  if (items.length === 0) return <EmptyState title="No subnets yet" />
  const max = Math.max(...items.map((i) => i.count))
  return (
    <ul aria-label={title} className="space-y-2 p-4">
      {items.map((item) => (
        <li key={item.label} title={`${item.label}: ${formatCount(item.count)}`}>
          <div className="mb-0.5 flex justify-between gap-3 text-[13px]">
            <span className="truncate text-ink">{item.label}</span>
            <span className="text-ink-2 tabular-nums">{formatCount(item.count)}</span>
          </div>
          <div className="h-2 rounded-r bg-surface-2"><div className="h-2 rounded-r bg-accent" style={{ width: `${Math.max((item.count / max) * 100, 1)}%` }} /></div>
        </li>
      ))}
    </ul>
  )
}
