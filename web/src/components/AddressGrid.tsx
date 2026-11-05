import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { get, list } from '../lib/api'
import { offsetIn } from '../lib/ip'
import type { GridCell, IpAddress, NetInfo } from '../lib/types'
import { allocationStates } from './inventory'
import { Button, EmptyState, ErrorState, Input, Mono, SkeletonRows } from './ui'

const pageSize = 256n

const kindStyle: Record<string, string> = {
  network: 'bg-[var(--line-strong)]',
  broadcast: 'bg-[var(--line-strong)]',
  gateway: 'bg-[var(--ink-2)]',
  reserved_range: 'bg-[repeating-linear-gradient(135deg,var(--line-strong)_0_2px,var(--surface-2)_2px_4px)]',
  dhcp: 'bg-[var(--map-a)]',
  candidate: 'bg-surface border border-[var(--state-available)]',
}
const stateStyle: Record<string, string> = {
  allocated: 'bg-[var(--state-allocated)]',
  reserved: 'bg-[var(--state-reserved)]',
  quarantined: 'bg-[var(--state-quarantined)]',
  retired: 'bg-[var(--state-retired)]',
  available_candidate: 'bg-[var(--state-available)]',
}

const kindLabel: Record<string, string> = {
  network: 'Network address', broadcast: 'Broadcast address', gateway: 'Gateway', reserved_range: 'Reserved range',
  dhcp: 'DHCP pool', candidate: 'Available candidate (no record)',
}

function describe(c: GridCell): string {
  if (c.kind !== 'record') return `${c.address}: ${kindLabel[c.kind]}`
  return `${c.address}: ${allocationStates[c.allocationState ?? ''] ?? c.allocationState}${c.hostname ? `, ${c.hostname}` : ''}`
}

function Legend() {
  const items: [string, string][] = [
    ['bg-[var(--state-allocated)]', 'Allocated'], ['bg-[var(--state-reserved)]', 'Reserved'],
    ['bg-[var(--state-quarantined)]', 'Quarantined'], ['bg-[var(--state-retired)]', 'Retired'],
    ['bg-[var(--state-available)]', 'Available candidate (record)'], [kindStyle.candidate, 'No record'],
    [kindStyle.reserved_range, 'Reserved range'], [kindStyle.dhcp, 'DHCP pool'], [kindStyle.gateway, 'Gateway'],
    [kindStyle.network, 'Network or broadcast'],
  ]
  return (
    <ul className="flex flex-wrap gap-x-3 gap-y-1 text-[12px] text-ink-2">
      {items.map(([cls, label]) => <li key={label} className="flex items-center gap-1.5"><span aria-hidden className={`size-3 rounded-[2px] ${cls}`} />{label}</li>)}
    </ul>
  )
}

/**
 * The subnet's address grid, one page of 256 at a time. Addresses without a
 * record are calculated by the API, so even a /64 never lists its range.
 */
export function AddressGrid({ subnetId, info, onRecord, onCandidate }:
  { subnetId: string; info: NetInfo; onRecord: (id: string) => void; onCandidate?: (address: string) => void }) {
  const total = BigInt(info.totalAddresses)
  const large = info.family === 6 && total > 4096n
  const [offset, setOffset] = useState<bigint | null>(large ? null : 0n)
  const [jump, setJump] = useState('')
  const [jumpError, setJumpError] = useState('')
  const [highlight, setHighlight] = useState('')

  const cells = useQuery({
    queryKey: ['subnets', subnetId, 'grid', offset?.toString()],
    enabled: offset !== null,
    placeholderData: keepPreviousData,
    queryFn: async () => (await list<GridCell>(`/subnets/${subnetId}/addresses?offset=${offset}&limit=${pageSize}`)).data,
  })
  const recorded = useQuery({
    queryKey: ['ip-addresses', 'subnet', subnetId, 'first'],
    enabled: large,
    queryFn: () => list<IpAddress>(`/ip-addresses?subnetId=${subnetId}&pageSize=50`),
  })

  const go = (target: bigint, address = '') => {
    setOffset(target - (target % pageSize))
    setHighlight(address)
  }
  const submit = (e: FormEvent) => {
    e.preventDefault()
    const off = offsetIn(info.cidr, jump)
    if (off === null) { setJumpError(`Enter an address inside ${info.cidr}.`); return }
    setJumpError('')
    go(off, jump.trim())
  }
  const last = offset === null ? 0n : offset + pageSize >= total ? total : offset + pageSize
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <form onSubmit={submit} className="flex items-start gap-2">
          <div>
            <Input aria-label="Jump to address" placeholder="Jump to address" className="w-56 font-mono" value={jump} onChange={(e) => setJump(e.target.value)} />
            {jumpError && <p role="alert" className="mt-1 text-[12.5px] text-danger">{jumpError}</p>}
          </div>
          <Button type="submit">Go</Button>
        </form>
        {offset !== null && (
          <div className="flex items-center gap-1 text-[12.5px] text-ink-2">
            <span className="tabular-nums">Positions {(offset + 1n).toLocaleString('en-GB')}–{last.toLocaleString('en-GB')} of {info.family === 6 && info.prefixLength < 96 ? `2^${128 - info.prefixLength}` : total.toLocaleString('en-GB')}</span>
            <Button variant="ghost" className="h-7 px-1" aria-label="Previous addresses" disabled={offset === 0n} onClick={() => go(offset - pageSize)}><ChevronLeft size={15} /></Button>
            <Button variant="ghost" className="h-7 px-1" aria-label="Next addresses" disabled={last >= total} onClick={() => go(offset + pageSize)}><ChevronRight size={15} /></Button>
          </div>
        )}
      </div>
      <Legend />
      {offset === null && (
        <div>
          <p className="mb-2 text-[13px] text-ink-2">This IPv6 subnet is too large to browse from the start. Choose a recorded address or jump to any address.</p>
          {recorded.isPending && <SkeletonRows rows={3} />}
          {recorded.error && <ErrorState error={recorded.error} onRetry={recorded.refetch} />}
          {recorded.data?.data.length === 0 && <EmptyState title="No recorded addresses yet" />}
          <ul className="grid gap-1 sm:grid-cols-2">
            {recorded.data?.data.map((a) => (
              <li key={a.id} className="flex items-center justify-between gap-2 rounded-[4px] border border-line px-2 py-1">
                <button type="button" className="cursor-pointer text-left hover:text-accent" onClick={() => onRecord(a.id)}><Mono>{a.address}</Mono></button>
                <Button variant="ghost" className="h-6 px-1.5 text-[12px]" onClick={() => go(offsetIn(info.cidr, a.address) ?? 0n, a.address)}>Show in grid</Button>
              </li>
            ))}
          </ul>
        </div>
      )}
      {offset !== null && cells.isPending && <SkeletonRows rows={4} />}
      {offset !== null && cells.error && <ErrorState error={cells.error} onRetry={cells.refetch} />}
      {offset !== null && cells.data && (
        <div role="grid" aria-label={`Addresses in ${info.cidr}`} className="grid grid-cols-[repeat(auto-fill,minmax(18px,1fr))] gap-[3px]">
          {cells.data.map((c) => {
            const style = c.kind === 'record' ? stateStyle[c.allocationState ?? ''] ?? 'bg-ink-3' : kindStyle[c.kind]
            const ring = c.address === highlight ? 'ring-2 ring-accent ring-offset-1' : ''
            const clickable = (c.kind === 'record' && c.id) || (c.kind === 'candidate' && onCandidate)
            const label = describe(c)
            return clickable ? (
              <button key={c.address} type="button" title={label} aria-label={label}
                onClick={() => (c.kind === 'record' && c.id ? onRecord(c.id) : onCandidate?.(c.address))}
                className={`aspect-square cursor-pointer rounded-[2px] hover:opacity-75 focus-visible:outline-2 focus-visible:outline-accent ${style} ${ring}`} />
            ) : <span key={c.address} role="gridcell" title={label} aria-label={label} className={`aspect-square rounded-[2px] ${style} ${ring}`} />
          })}
        </div>
      )}
    </div>
  )
}

/** Loads one record for detail views. */
export const fetchAddress = (id: string) => get<IpAddress>(`/ip-addresses/${id}`)
