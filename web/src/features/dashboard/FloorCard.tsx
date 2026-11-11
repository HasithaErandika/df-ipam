import { ChevronRight } from 'lucide-react'
import { Link } from 'react-router'
import { Meter } from '../../components/ui'
import { formatCount, level, segmentCaption } from '../../lib/format'
import type { Dashboard } from '../../lib/types'

export function FloorCard({ floor: f }: { floor: Dashboard['floors'][number] }) {
  const pct = f.poolSize ? Math.min(100, (f.poolUsed * 100) / f.poolSize) : 0
  return (
    <Link to={`/segments/${f.id}`} className="group rounded-md border border-line p-3.5 transition-colors hover:border-accent/50 hover:bg-accent-soft/20">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <h3 className="truncate text-[13.5px] font-semibold text-ink">{f.name}</h3>
          <p className="truncate font-mono text-[11.5px] text-ink-3">{segmentCaption(f)}</p>
        </div>
        <ChevronRight size={15} className="mt-0.5 shrink-0 text-ink-3 group-hover:text-accent" aria-hidden />
      </div>
      <div className="mt-3">
        <div className="mb-1 flex justify-between text-[12px] text-ink-3">
          <span>DHCP pool</span>
          <span className="tabular-nums">{f.poolSize ? `${formatCount(f.poolUsed)} of ${formatCount(f.poolSize)}` : 'No pool set'}</span>
        </div>
        <Meter wide pct={pct} level={level(pct, 75, 90)} label={`${f.name} DHCP pool use`} />
      </div>
      <dl className="mt-3 grid grid-cols-3 gap-2 border-t border-line pt-2.5 text-center">
        {([['Seen', f.seen], ['Static', f.static], ['Registered', f.registered]] as const).map(([k, v]) => (
          <div key={k}><dt className="text-[11px] text-ink-3">{k}</dt><dd className="text-[14px] font-semibold text-ink tabular-nums">{formatCount(v)}</dd></div>
        ))}
      </dl>
    </Link>
  )
}
