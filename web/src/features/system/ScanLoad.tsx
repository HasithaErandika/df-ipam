import { formatCount } from '../../lib/format'
import { useScanStatus } from '../../lib/scan'

/** Rough bytes on the wire per address probed: a ping and its reply, plus a share of NetBIOS and DNS. */
const bytesPerProbe = 400

const duration = (seconds: number) => (seconds < 90 ? `${Math.max(1, Math.round(seconds))} s` : `${Math.round(seconds / 60)} min`)

/**
 * What the scan settings cost, for the values being edited: how long one round
 * takes and how much traffic it makes. Estimates from the segments' sizes.
 */
export function ScanLoad({ rate, userMinutes, serverMinutes }: { rate: number; userMinutes: number; serverMinutes: number }) {
  const q = useScanStatus()
  // Nothing while a value is being typed or out of range.
  if (!q.data || !(rate > 0) || !(userMinutes > 0) || !(serverMinutes > 0)) return null
  const { userAddresses: users, serverAddresses: servers } = q.data
  const total = users + servers
  if (total === 0) return <p className="border-t border-line px-4 py-3 text-[12.5px] text-ink-3">No segments to scan yet. The estimate appears once segments are added.</p>
  const mbit = (rate * bytesPerProbe * 8) / 1e6
  return (
    <div className="border-t border-line px-4 py-3 text-[12.5px] text-ink-2">
      <span className="font-medium text-ink">Expected load: </span>
      {formatCount(users)} user-network addresses take about {duration(users / rate)} every {userMinutes} min, and {formatCount(servers)} server-network addresses about {duration(servers / rate)} every {serverMinutes} min.
      {' '}Traffic stays steady at about {mbit < 0.1 ? 'under 0.1' : mbit.toFixed(1)} Mbit/s while a scan runs, with no bursts.
    </div>
  )
}
