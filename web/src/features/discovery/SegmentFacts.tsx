import { Copy, Plus } from 'lucide-react'
import { addressStates } from '../../components/charts'
import { Button, Card, Details, toast } from '../../components/ui'
import { formatCount } from '../../lib/format'
import type { Segment } from '../../lib/types'
import { ipToInt } from '../../lib/validation'

export interface NetworkInfo { network: string; prefixLength: number; mask?: string; broadcast?: string; firstUsable: string; lastUsable: string; totalAddresses: string; usableAddresses: string }
export interface FreeRange { start: string; end: string; count: number }

const order = ['assigned', 'reserved', 'blocked', 'held', 'gateway', 'seen', 'dhcp_used', 'dhcp_free', 'available']
const pct = (n: number, of: number) => (of ? `${((n * 100) / of).toFixed(n * 100 < of ? 1 : 0)}%` : '0%')

/** The subnet maths for a segment. */
export function NetworkFacts({ segment: s, info }: { segment: Segment; info: NetworkInfo }) {
  const pool = s.dhcpStart && s.dhcpEnd ? ipToInt(s.dhcpEnd) - ipToInt(s.dhcpStart) + 1 : 0
  return (
    <Card title="Network">
      <div className="p-4">
        <Details items={[
          ['Network', <span key="n" className="font-mono">{info.network}/{info.prefixLength}</span>],
          ['Subnet mask', <span key="m" className="font-mono">{info.mask ?? '—'}</span>],
          ['Usable range', <span key="u" className="font-mono">{info.firstUsable} – {info.lastUsable}</span>],
          ['Broadcast', <span key="b" className="font-mono">{info.broadcast ?? '—'}</span>],
          ['Addresses', `${formatCount(Number(info.totalAddresses))} total, ${formatCount(Number(info.usableAddresses))} usable`],
          ['Gateway', s.gateway ? <span key="g" className="font-mono">{s.gateway}</span> : s.detectedGateway ? <span key="g"><span className="font-mono">{s.detectedGateway}</span> <span className="text-ink-3">(detected)</span></span> : 'Not set'],
          ['DHCP pool', pool ? <span key="p"><span className="font-mono">{s.dhcpStart} – {s.dhcpEnd}</span> <span className="text-ink-3">· {formatCount(pool)}</span></span> : s.kind === 'dhcp' ? 'Not set' : 'None (static only)'],
          ['VLAN', s.vlan ?? '—'],
        ]} />
      </div>
    </Card>
  )
}

/** How every usable address is used, as counts and shares. */
export function UsageBreakdown({ counts, usable }: { counts: Record<string, number>; usable: number }) {
  return (
    <Card title="How it's used" subtitle={`${formatCount(usable)} usable addresses`}>
      <ul className="divide-y divide-line">
        {order.map((k) => (
          <li key={k} className="flex items-center gap-3 px-4 py-2 text-[13px]">
            <span aria-hidden className="size-2.5 shrink-0 rounded-[2px]" style={{ backgroundColor: addressStates[k].color }} />
            <span className="min-w-0 flex-1 truncate text-ink-2" title={addressStates[k].hint}>{addressStates[k].label}</span>
            <span className="font-medium text-ink tabular-nums">{formatCount(counts[k] ?? 0)}</span>
            <span className="w-12 text-right text-[12px] text-ink-3 tabular-nums">{pct(counts[k] ?? 0, usable)}</span>
          </li>
        ))}
      </ul>
    </Card>
  )
}

/** Every run of Available candidates, with a quick way to register one. */
export function AvailableList({ segment: s, ranges, onRegister }: { segment: Segment; ranges: FreeRange[]; onRegister?: (ip: string) => void }) {
  const total = ranges.reduce((n, r) => n + r.count, 0)
  const copy = (r: FreeRange) => {
    void navigator.clipboard?.writeText(r.start === r.end ? r.start : `${r.start}-${r.end}`).then(() => toast.success('Copied.'))
  }
  return (
    <Card title="Available addresses" subtitle={total ? `${formatCount(total)} free for static use, in ${ranges.length} range${ranges.length === 1 ? '' : 's'}` : undefined}>
      {total === 0 ? (
        <div className="p-4 text-[13px] text-ink-2">
          <p className="font-medium text-ink">No Available candidates.</p>
          <p className="mt-1">{s.kind === 'dhcp' && s.dhcpStart
            ? 'Every usable address is in the DHCP pool, registered, reserved or in use. To assign static addresses, shrink the DHCP pool in Settings > Segments (and on the DHCP server) to leave a static range.'
            : 'Every usable address is registered, reserved or in use. Release or reclaim addresses, or widen the segment.'}</p>
        </div>
      ) : (
        <ul className="max-h-80 divide-y divide-line overflow-y-auto">
          {ranges.map((r) => (
            <li key={r.start} className="flex items-center gap-2 px-4 py-2">
              <span className="min-w-0 flex-1 font-mono text-[12.5px] text-ink">{r.start === r.end ? r.start : `${r.start} – ${r.end}`}</span>
              <span className="text-[12px] text-ink-3 tabular-nums">{formatCount(r.count)}</span>
              <button type="button" title="Copy" aria-label={`Copy ${r.start}`} onClick={() => copy(r)} className="grid size-7 cursor-pointer place-items-center rounded-[4px] text-ink-3 hover:bg-surface-2 hover:text-ink"><Copy size={13} aria-hidden /></button>
              {onRegister && <Button className="h-7" onClick={() => onRegister(r.start)}><Plus size={13} aria-hidden />{r.start.split('.').pop()}</Button>}
            </li>
          ))}
        </ul>
      )}
      <p className="border-t border-line px-4 py-2 text-[12px] text-ink-3">Not registered, not reserved, not the gateway, outside the DHCP pool, and not seen in use in the last 24 hours.</p>
    </Card>
  )
}
