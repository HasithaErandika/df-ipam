import { useState } from 'react'
import { formatCount } from '../lib/format'
import { intToIP, ipToInt } from '../lib/validation'

export interface Part { key: string; label: string; value: number; color: string }

/**
 * One horizontal bar split into parts, with a legend that names each part and
 * its count. Hovering a part shows its share. Identity never rests on colour
 * alone: every part is named in the legend with its number.
 */
export function StackedBar({ parts, label, height = 12 }: { parts: Part[]; label: string; height?: number }) {
  const [hover, setHover] = useState<string | null>(null)
  const total = parts.reduce((n, p) => n + p.value, 0)
  const shown = parts.filter((p) => p.value > 0)
  const focus = shown.find((p) => p.key === hover)
  return (
    <figure className="m-0">
      <figcaption className="sr-only">{label}: {parts.map((p) => `${p.label} ${p.value}`).join(', ')}</figcaption>
      <div className="flex w-full gap-[2px] overflow-hidden rounded-[4px] bg-track" style={{ height }} onMouseLeave={() => setHover(null)}>
        {total === 0 ? null : shown.map((p) => (
          <div key={p.key} onMouseEnter={() => setHover(p.key)} className="h-full first:rounded-l-[4px] last:rounded-r-[4px] transition-opacity"
            style={{ width: `${(p.value / total) * 100}%`, minWidth: 3, backgroundColor: p.color, opacity: hover && hover !== p.key ? 0.45 : 1 }} />
        ))}
      </div>
      <div className="mt-1 h-4 text-[12px] text-ink-2" aria-hidden>
        {focus ? <><strong className="font-medium text-ink">{focus.label}</strong> · {formatCount(focus.value)} ({total ? Math.round((focus.value / total) * 100) : 0}%)</> : null}
      </div>
      <Legend parts={parts} onHover={setHover} />
    </figure>
  )
}

/** Each part with its colour, count and share of the whole. */
export function Legend({ parts, onHover }: { parts: Part[]; onHover?: (key: string | null) => void }) {
  const total = parts.reduce((n, p) => n + p.value, 0)
  return (
    <ul className="grid grid-cols-2 gap-x-4 gap-y-2.5 sm:grid-cols-3 lg:grid-cols-5">
      {parts.map((p) => (
        <li key={p.key} className="min-w-0" onMouseEnter={() => onHover?.(p.key)} onMouseLeave={() => onHover?.(null)}>
          <div className="flex items-center gap-1.5 text-[12px] text-ink-2">
            <span aria-hidden className="size-2.5 shrink-0 rounded-[2px]" style={{ backgroundColor: p.color }} />
            <span className="truncate">{p.label}</span>
          </div>
          <div className="mt-0.5 pl-4 tabular-nums">
            <span className="text-[14px] font-semibold text-ink">{formatCount(p.value)}</span>
            <span className="ml-1.5 text-[12px] text-ink-3">{share(p.value, total)}</span>
          </div>
        </li>
      ))}
    </ul>
  )
}

/** A share as a percentage, with one decimal below 10% so small parts don't read as 0%. */
function share(n: number, total: number): string {
  if (!total || !n) return '0%'
  const pct = (n * 100) / total
  return pct < 10 ? `${pct.toFixed(1)}%` : `${Math.round(pct)}%`
}

export const addressStates: Record<string, { label: string; color: string; hint: string }> = {
  assigned: { label: 'Assigned', color: 'var(--addr-assigned)', hint: 'In the register, assigned to someone or something.' },
  reserved: { label: 'Reserved', color: 'var(--addr-reserved)', hint: 'In the register, kept for later.' },
  blocked: { label: 'Blocked', color: 'var(--addr-blocked)', hint: 'In the register, never to be used.' },
  held: { label: 'Held', color: 'var(--addr-held)', hint: 'Inside an active reservation.' },
  gateway: { label: 'Gateway', color: 'var(--addr-gateway)', hint: "The segment's gateway." },
  seen: { label: 'Seen, not registered', color: 'var(--addr-seen)', hint: 'Answered on the network but is not in the register.' },
  dhcp_used: { label: 'DHCP in use', color: 'var(--addr-dhcp-used)', hint: 'In the DHCP pool and answering.' },
  dhcp_free: { label: 'DHCP free', color: 'var(--addr-dhcp-free)', hint: 'In the DHCP pool, not seen recently.' },
  available: { label: 'Available candidate', color: 'var(--addr-available)', hint: 'Not registered, reserved, in the pool or seen.' },
}

export interface Run { start: string; end: string; state: string; count: number }

/**
 * Every usable address of a segment as a cell, filling the width: 16 to a row on
 * phones, 32 on tablets, 64 on wide screens, so rows stay aligned to address
 * boundaries. Hovering names the address; clicking hands it to the page. The
 * address list below the map gives the same information as a table.
 */
export function AddressMap({ runs, onPick }: { runs: Run[]; onPick?: (ip: string, state: string) => void }) {
  const [hover, setHover] = useState<{ ip: string; state: string } | null>(null)
  const cells: { ip: string; state: string }[] = []
  for (const r of runs) for (let n = ipToInt(r.start); n <= ipToInt(r.end); n++) cells.push({ ip: intToIP(n), state: r.state })
  return (
    <div>
      <div className="mb-2 h-5 truncate text-[12.5px] text-ink-2" aria-live="polite">
        {hover
          ? <><span className="font-mono font-medium text-ink">{hover.ip}</span> · {addressStates[hover.state]?.label}<span className="text-ink-3"> · {addressStates[hover.state]?.hint}</span></>
          : <span className="text-ink-3">Hover an address to see its state{onPick ? '; click an available or unregistered one to register it' : ''}.</span>}
      </div>
      <div role="img" aria-label={`Address map of ${cells.length} addresses`} onMouseLeave={() => setHover(null)}
        className="grid w-full grid-cols-[repeat(16,minmax(0,1fr))] gap-[2px] md:grid-cols-[repeat(32,minmax(0,1fr))] xl:grid-cols-[repeat(64,minmax(0,1fr))]">
        {cells.map((c) => {
          const actionable = onPick && (c.state === 'available' || c.state === 'seen')
          return (
            <div key={c.ip} title={`${c.ip} · ${addressStates[c.state]?.label ?? c.state}`}
              onMouseEnter={() => setHover(c)} onClick={() => onPick?.(c.ip, c.state)}
              className={`aspect-square min-h-2 rounded-[3px] transition-[outline] ${actionable ? 'cursor-pointer' : ''} ${hover?.ip === c.ip ? 'outline-2 outline-offset-1 outline-ink' : ''}`}
              style={{ backgroundColor: addressStates[c.state]?.color ?? 'var(--track)' }} />
          )
        })}
      </div>
    </div>
  )
}
