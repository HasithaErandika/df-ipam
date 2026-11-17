import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, CheckCircle2, Loader2 } from 'lucide-react'
import { get, queryString } from '../../lib/api'
import { useDebounced } from '../../lib/listing'
import type { AddressStatus } from '../../lib/types'
import { isIPv4 } from '../../lib/validation'

/** Problems that stop an assignment, and ones that only need a second look. */
export function addressProblems(s: AddressStatus): { blocking: string[]; warnings: string[] } {
  const blocking: string[] = []
  const warnings: string[] = []
  if (!s.inSegment) blocking.push('Not inside this segment.')
  else if (!s.usable) blocking.push('This is the network or broadcast address.')
  if (s.isGateway) blocking.push("This is the segment's gateway.")
  if (s.registered) blocking.push(`Already in the register${s.registerName ? ` as “${s.registerName}”` : ''}.`)
  if (s.reserved) blocking.push('Held by a reservation.')
  if (s.inDhcpPool) warnings.push('Inside the DHCP pool: a DHCP client may be given this address.')
  if (s.seenRecently) warnings.push(`Seen in use recently${s.seenMac ? ` by ${s.seenMac}` : ''}${s.hostname ? ` (${s.hostname})` : ''}.`)
  return { blocking, warnings }
}

/**
 * Checks an address as it is typed and says what already holds or uses it. The
 * API checks again on save; this lets a person see a clash before they submit.
 */
export function AddressCheck({ segmentId, ip, allowSeen = true }: { segmentId: string; ip: string; allowSeen?: boolean }) {
  const settled = useDebounced(ip.trim(), 400)
  const ready = Boolean(segmentId) && isIPv4(settled)
  const query = useQuery({
    queryKey: ['/ip-addresses/check', segmentId, settled],
    queryFn: () => get<{ status: AddressStatus; available: boolean }>(`/ip-addresses/check${queryString({ segmentId, ip: settled })}`),
    enabled: ready,
    staleTime: 10_000,
  })
  if (!ready) return null
  if (query.isPending) return <p className="flex items-center gap-1.5 text-[12.5px] text-ink-3"><Loader2 size={13} className="animate-spin" aria-hidden />Checking {settled}…</p>
  if (query.isError) return null
  const { blocking, warnings } = addressProblems(query.data.status)
  if (!allowSeen && query.data.status.seenRecently) blocking.push(...warnings.splice(warnings.findIndex((w) => w.startsWith('Seen')), 1))
  if (blocking.length === 0 && warnings.length === 0) {
    return <p className="flex items-center gap-1.5 text-[12.5px] text-ok"><CheckCircle2 size={13} aria-hidden />{settled} is an Available candidate.</p>
  }
  return (
    <ul className="space-y-1" aria-live="polite">
      {blocking.map((m) => <li key={m} className="flex gap-1.5 text-[12.5px] text-danger"><AlertTriangle size={13} className="mt-0.5 shrink-0" aria-hidden />{m}</li>)}
      {warnings.map((m) => <li key={m} className="flex gap-1.5 text-[12.5px] text-warn"><AlertTriangle size={13} className="mt-0.5 shrink-0" aria-hidden />{m}</li>)}
    </ul>
  )
}
