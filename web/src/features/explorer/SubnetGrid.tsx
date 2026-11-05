import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useState } from 'react'
import { allocationStates, observationStates } from '../../components/devices'
import { Button, ErrorState, SkeletonRows } from '../../components/ui'
import { getBody } from '../../lib/api'
import type { GridCell, Subnet } from '../../lib/types'

const cellColour: Record<string, string> = {
  allocated: 'bg-[var(--state-allocated)] text-white', reserved: 'bg-[var(--state-reserved)] text-white',
  quarantined: 'bg-[var(--state-quarantined,#d6a400)] text-white', retired: 'bg-track text-ink-3',
  available_candidate: 'bg-ok-soft text-ok', candidate: 'bg-surface text-ink-3',
  network: 'bg-track text-ink-3', broadcast: 'bg-track text-ink-3', gateway: 'bg-accent-soft font-semibold text-accent',
  reserved_range: 'bg-warn-soft text-warn', dhcp: 'bg-[repeating-linear-gradient(135deg,var(--surface-2),var(--surface-2)_4px,var(--track)_4px,var(--track)_6px)] text-ink-2',
}

const gridLegend: [string, string][] = [['allocated', 'Allocated'], ['reserved', 'Reserved'], ['available_candidate', 'Recorded, available'], ['candidate', 'Available candidate'], ['gateway', 'Gateway'], ['reserved_range', 'Reserved range'], ['dhcp', 'DHCP pool']]

/** A page of the subnet's addresses. Large and IPv6 subnets load one range at a time. */
export function SubnetGrid({ subnet, onOpen }: { subnet: Subnet; onOpen: (id: string) => void }) {
  const limit = 256
  const [offset, setOffset] = useState(0n)
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['ip', 'grid', subnet.id, offset.toString()], placeholderData: keepPreviousData,
    queryFn: () => getBody<{ data: GridCell[]; meta: { offset: string; limit: number; total: string } }>(`/subnets/${subnet.id}/addresses?offset=${offset}&limit=${limit}`),
  })
  const total = data ? BigInt(data.meta.total) : 0n
  return (
    <div className="p-3">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2 text-[12.5px] text-ink-2">
        <div className="flex flex-wrap gap-x-3 gap-y-1">{gridLegend.map(([k, label]) => <span key={k} className="flex items-center gap-1"><span className={`inline-block size-3 rounded-[2px] border border-line ${cellColour[k]}`} />{label}</span>)}</div>
        <div className="flex items-center gap-1">
          <span className="tabular-nums">{data && `${data.data[0]?.address ?? ''} – ${data.data[data.data.length - 1]?.address ?? ''}`}</span>
          <Button variant="ghost" className="h-7 px-1" aria-label="Previous range" disabled={offset === 0n} onClick={() => setOffset(offset - BigInt(limit) < 0n ? 0n : offset - BigInt(limit))}><ChevronLeft size={15} /></Button>
          <Button variant="ghost" className="h-7 px-1" aria-label="Next range" disabled={offset + BigInt(limit) >= total} onClick={() => setOffset(offset + BigInt(limit))}><ChevronRight size={15} /></Button>
        </div>
      </div>
      {isPending && <SkeletonRows rows={4} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data && (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(4.5rem,1fr))] gap-1">
          {data.data.map((cell) => {
            const key = cell.kind === 'record' ? cell.allocationState ?? 'candidate' : cell.kind
            const last = cell.address.split(/[.:]/).pop()
            const label = `${cell.address}: ${cell.kind === 'record' ? `${allocationStates[cell.allocationState ?? '']?.label ?? ''}, ${observationStates[cell.observationState ?? '']?.label ?? ''}${cell.hostname ? `, ${cell.hostname}` : ''}` : cell.kind.replace('_', ' ')}`
            return (
              <button key={cell.address} type="button" title={label} aria-label={label} disabled={!cell.id} onClick={() => cell.id && onOpen(cell.id)}
                className={`h-9 truncate rounded-[3px] border border-line px-1 text-left font-mono text-[11px] ${cellColour[key] ?? 'bg-surface'} ${cell.id ? 'cursor-pointer hover:ring-2 hover:ring-accent/40' : 'cursor-default'} ${cell.observationState === 'conflict_suspected' ? 'ring-2 ring-danger' : ''}`}>
                .{last}{cell.hostname && <span className="block truncate font-sans text-[10px] opacity-80">{cell.hostname}</span>}
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}
