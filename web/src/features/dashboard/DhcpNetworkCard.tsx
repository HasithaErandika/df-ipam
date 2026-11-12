import { ChevronRight, Wifi } from 'lucide-react'
import { Link } from 'react-router'
import { Meter } from '../../components/ui'
import { formatCount, level, segmentCaption } from '../../lib/format'
import type { Dashboard } from '../../lib/types'

/** One DHCP network (a floor or Wi-Fi): pool use and what is on it now. */
export function DhcpNetworkCard({ network: f }: { network: Dashboard['floors'][number] }) {
  const pct = f.poolSize ? Math.min(100, (f.poolUsed * 100) / f.poolSize) : 0
  return (
    <Link to={`/segments/${f.id}`} className="group rounded-lg border border-line bg-surface p-4 transition-colors hover:border-accent/50 hover:shadow-[0_4px_16px_-8px_rgba(11,18,32,.18)]">
      <div className="flex items-start gap-3">
        <span aria-hidden className="grid size-9 shrink-0 place-items-center rounded-md bg-accent-soft text-accent"><Wifi size={17} /></span>
        <div className="min-w-0 flex-1">
          <h3 className="truncate text-[14px] font-semibold text-ink">{f.name}</h3>
          <p className="truncate font-mono text-[11.5px] text-ink-3">{segmentCaption(f)}</p>
        </div>
        <ChevronRight size={16} className="mt-1 shrink-0 text-ink-3 group-hover:text-accent" aria-hidden />
      </div>
      <div className="mt-3.5">
        <div className="mb-1 flex justify-between text-[12px] text-ink-3">
          <span>DHCP pool in use</span>
          <span className="tabular-nums">{f.poolSize ? `${formatCount(f.poolUsed)} of ${formatCount(f.poolSize)}` : 'No pool set'}</span>
        </div>
        <Meter wide pct={pct} level={level(pct, 75, 90)} label={`${f.name} DHCP pool use`} />
      </div>
      <dl className="mt-3 grid grid-cols-3 gap-2 border-t border-line pt-2.5 text-center">
        {([['Seen now', f.seen], ['Static', f.static], ['Registered', f.registered]] as const).map(([k, v]) => (
          <div key={k}><dt className="text-[11px] text-ink-3">{k}</dt><dd className="text-[15px] font-semibold text-ink tabular-nums">{formatCount(v)}</dd></div>
        ))}
      </dl>
    </Link>
  )
}
