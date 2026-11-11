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

export function Legend({ parts, onHover }: { parts: Part[]; onHover?: (key: string | null) => void }) {
  return (
    <ul className="flex flex-wrap gap-x-4 gap-y-1">
      {parts.map((p) => (
        <li key={p.key} className="flex items-center gap-1.5 text-[12px] text-ink-2" onMouseEnter={() => onHover?.(p.key)} onMouseLeave={() => onHover?.(null)}>
          <span aria-hidden className="size-2.5 rounded-[2px]" style={{ backgroundColor: p.color }} />
          {p.label}<span className="font-medium text-ink tabular-nums">{formatCount(p.value)}</span>
        </li>
      ))}
    </ul>
  )
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
 * Every usable address of a segment as a cell, 32 to a row, coloured by state.
 * Hovering names the address; clicking hands it to the page.
 */
export function AddressMap({ runs, onPick }: { runs: Run[]; onPick?: (ip: string, state: string) => void }) {
  const [hover, setHover] = useState<{ ip: string; state: string } | null>(null)
  const cells: { ip: string; state: string }[] = []
  for (const r of runs) for (let n = ipToInt(r.start); n <= ipToInt(r.end); n++) cells.push({ ip: intToIP(n), state: r.state })
  const cols = 32
  const size = 14
  const gap = 2
  const rows = Math.ceil(cells.length / cols)
  return (
    <div>
      <div className="mb-2 h-5 text-[12.5px] text-ink-2" aria-live="polite">
        {hover
          ? <><span className="font-mono font-medium text-ink">{hover.ip}</span> · {addressStates[hover.state]?.label}<span className="text-ink-3"> · {addressStates[hover.state]?.hint}</span></>
          : <span className="text-ink-3">Hover an address to see its state{onPick ? '; click to act on it' : ''}.</span>}
      </div>
      <div className="overflow-x-auto">
        <svg role="img" aria-label={`Address map of ${cells.length} addresses`} width={cols * (size + gap)} height={rows * (size + gap)} onMouseLeave={() => setHover(null)}>
          {cells.map((c, i) => (
            <rect key={c.ip} x={(i % cols) * (size + gap)} y={Math.floor(i / cols) * (size + gap)} width={size} height={size} rx={3}
              fill={addressStates[c.state]?.color ?? 'var(--track)'} stroke={hover?.ip === c.ip ? 'var(--ink)' : 'none'} strokeWidth={1.5}
              className={onPick ? 'cursor-pointer' : undefined} onMouseEnter={() => setHover(c)} onClick={() => onPick?.(c.ip, c.state)}>
              <title>{`${c.ip} · ${addressStates[c.state]?.label ?? c.state}`}</title>
            </rect>
          ))}
        </svg>
      </div>
    </div>
  )
}
