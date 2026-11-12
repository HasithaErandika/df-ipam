import type { ReactNode } from 'react'
import { Details } from '../../components/ui'
import { formatAgo, formatDateTime } from '../../lib/format'
import { deviceFacts, networkCard, serviceName, type Evidence } from '../discovery/deviceInfo'

/** A titled block inside a details dialog. */
export function Section({ title, children }: { title: string; children: ReactNode }) {
  return <section><h3 className="mb-2 text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase">{title}</h3>{children}</section>
}

/** A time with how long ago, or nothing. */
export const when = (t: string | null | undefined) => (t ? <span title={formatDateTime(t)}>{formatDateTime(t)} ({formatAgo(t)})</span> : null)

/** Open ports with their usual service. */
export function PortList({ ports, checked = true }: { ports: number[]; checked?: boolean }) {
  if (!checked) return <p className="text-[12.5px] text-ink-3">Not checked on this segment.</p>
  if (ports.length === 0) return <p className="text-[12.5px] text-ink-3">None of the common ports answered.</p>
  return (
    <ul className="flex flex-wrap gap-1.5">{ports.map((p) => (
      <li key={p} className="inline-flex items-center gap-1.5 rounded-[4px] border border-line bg-surface-2 px-2 py-0.5 text-[12.5px]">
        <span className="font-mono text-ink">{p}</span><span className="text-ink-2">{serviceName(p)}</span>
      </li>
    ))}</ul>
  )
}

/** What the scanner says the device is, and its network card. */
export function EvidenceDetails({ e, mac, privateMac }: { e: Evidence; mac: string | null; privateMac?: boolean }) {
  const f = deviceFacts(e)
  const card = networkCard(e)
  return (
    <Details items={[
      ['Maker', f.maker ? <span>{f.maker} <span className="text-ink-3">({f.makerFrom.toLowerCase()})</span></span> : null],
      ['Model', f.model || null],
      ['Kind', f.kind || null],
      ['Likely system', f.system || null],
      ['Hostname', e.hostname || null],
      ['MAC address', mac ? <span className="font-mono">{mac}{privateMac ? ' (private, random)' : ''}</span> : 'Not seen'],
      ['Network card maker', card || (privateMac ? 'Hidden by a private MAC' : null)],
      ...(f.dhcpClass ? [['DHCP vendor class', f.dhcpClass] as [string, string]] : []),
    ]} />
  )
}
