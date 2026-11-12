import { Copy, Plus } from 'lucide-react'
import { Button, Card, toast } from '../../components/ui'
import { formatCount } from '../../lib/format'
import type { Segment } from '../../lib/types'
import { ipToInt } from '../../lib/validation'

export interface NetworkInfo { network: string; prefixLength: number; mask?: string; broadcast?: string; firstUsable: string; lastUsable: string; totalAddresses: string; usableAddresses: string }
export interface FreeRange { start: string; end: string; count: number }


function Tile({ label, value, sub }: { label: string; value: React.ReactNode; sub?: React.ReactNode }) {
  return (
    <div className="min-w-0 rounded-md bg-surface-2 px-3 py-2.5">
      <div className="text-[11.5px] text-ink-3">{label}</div>
      <div className="mt-0.5 truncate text-[14px] font-semibold text-ink">{value}</div>
      {sub ? <div className="truncate text-[11.5px] text-ink-3">{sub}</div> : null}
    </div>
  )
}

/**
 * Where the parts of the network sit, drawn to scale: the network and broadcast
 * addresses at the ends, the usable range between, the DHCP pool over it and
 * the gateway as a marker.
 */
function AddressLayout({ info, poolStart, poolEnd, gateway }: { info: NetworkInfo; poolStart: string | null; poolEnd: string | null; gateway: string | null }) {
  const base = ipToInt(info.network)
  const span = Number(info.totalAddresses) - 1 || 1
  const at = (ip: string) => ((ipToInt(ip) - base) / span) * 100
  const last = (ip: string) => ip.split('.').pop()
  return (
    <div>
      <div className="relative h-7 overflow-hidden rounded-[5px] bg-track">
        <div className="absolute inset-y-0 left-0 w-[3px] bg-ink-3" title={`Network address ${info.network}`} />
        <div className="absolute inset-y-0 right-0 w-[3px] bg-ink-3" title={`Broadcast ${info.broadcast}`} />
        {poolStart && poolEnd && (
          <div className="absolute inset-y-1 rounded-[3px] bg-[var(--addr-dhcp-free)] ring-1 ring-[var(--addr-dhcp-used)]/40"
            style={{ left: `${at(poolStart)}%`, width: `${Math.max(at(poolEnd) - at(poolStart), 0.8)}%` }} title={`DHCP pool ${poolStart} – ${poolEnd}`} />
        )}
        {gateway && (
          <div className="absolute inset-y-0 w-[3px] -translate-x-1/2 bg-[var(--addr-gateway)]" style={{ left: `${at(gateway)}%` }} title={`Gateway ${gateway}`} />
        )}
      </div>
      <div className="mt-1 flex justify-between font-mono text-[11px] text-ink-3">
        <span>.{last(info.network)} network</span>
        <span>.{last(info.firstUsable)} – .{last(info.lastUsable)} usable</span>
        <span>.{last(info.broadcast ?? info.lastUsable)} broadcast</span>
      </div>
      <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-[11.5px] text-ink-2">
        {poolStart && <span className="flex items-center gap-1.5"><span aria-hidden className="size-2.5 rounded-[2px] bg-[var(--addr-dhcp-free)] ring-1 ring-[var(--addr-dhcp-used)]/40" />DHCP pool</span>}
        {gateway && <span className="flex items-center gap-1.5"><span aria-hidden className="h-2.5 w-[3px] bg-[var(--addr-gateway)]" />Gateway</span>}
        <span className="flex items-center gap-1.5"><span aria-hidden className="h-2.5 w-[3px] bg-ink-3" />Network and broadcast</span>
      </div>
    </div>
  )
}

/** The subnet maths for a segment. */
export function NetworkFacts({ segment: s, info }: { segment: Segment; info: NetworkInfo }) {
  const pool = s.dhcpStart && s.dhcpEnd ? ipToInt(s.dhcpEnd) - ipToInt(s.dhcpStart) + 1 : 0
  const usable = Number(info.usableAddresses)
  const gateway = s.gateway ?? s.detectedGateway
  return (
    <Card title="Network">
      <div className="space-y-4 p-4">
        <div>
          <p className="font-mono text-[22px] leading-tight font-semibold text-ink">{info.network}<span className="text-ink-3">/{info.prefixLength}</span></p>
          <p className="mt-0.5 text-[12.5px] text-ink-3">Subnet mask <span className="font-mono text-ink-2">{info.mask}</span> · usable <span className="font-mono text-ink-2">{info.firstUsable} – {info.lastUsable}</span></p>
        </div>
        <AddressLayout info={info} poolStart={s.dhcpStart} poolEnd={s.dhcpEnd} gateway={gateway} />
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
          <Tile label="Total addresses" value={formatCount(Number(info.totalAddresses))} sub="including network and broadcast" />
          <Tile label="Usable" value={formatCount(usable)} sub={`.${info.firstUsable.split('.').pop()} to .${info.lastUsable.split('.').pop()}`} />
          <Tile label="DHCP pool" value={pool ? formatCount(pool) : s.kind === 'dhcp' ? 'Not set' : 'None'}
            sub={pool ? `${Math.round((pool * 100) / usable)}% of usable` : s.kind === 'dhcp' ? 'Set it in Network setup' : 'Static addresses only'} />
          <Tile label="Gateway" value={gateway ? <span className="font-mono">{gateway}</span> : 'Not set'} sub={!s.gateway && s.detectedGateway ? 'Detected, not saved' : s.gateway ? 'Saved on the segment' : undefined} />
          <Tile label="Broadcast" value={<span className="font-mono">{info.broadcast ?? '—'}</span>} />
          <Tile label="VLAN" value={s.vlan ?? '—'} sub={s.floor !== null ? `Floor ${s.floor}` : undefined} />
        </div>
      </div>
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
            ? 'Every usable address is in the DHCP pool, registered, reserved or in use. To assign static addresses, shrink the DHCP pool in Network setup > Segments (and on the DHCP server) to leave a static range.'
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
