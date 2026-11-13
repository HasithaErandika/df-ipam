import { AlertTriangle, Cloud, Server, Wifi, type LucideIcon } from 'lucide-react'
import { Link } from 'react-router'
import { Card, EmptyState, ErrorState, SkeletonRows } from '../../components/ui'
import { formatCount } from '../../lib/format'
import { useSegments } from '../../lib/queries'
import type { Segment } from '../../lib/types'
import { attention } from '../network/segmentList'

const kinds: { kind: Segment['kind']; title: string; icon: LucideIcon; tint: string }[] = [
  { kind: 'dhcp', title: 'DHCP networks', icon: Wifi, tint: 'bg-accent-soft text-accent' },
  { kind: 'server', title: 'Server networks', icon: Server, tint: 'bg-ok-soft text-ok' },
  { kind: 'cloud', title: 'Cloud networks', icon: Cloud, tint: 'bg-surface-2 text-ink-2' },
]

function Tile({ kind, title, icon: Icon, tint, rows }: (typeof kinds)[number] & { rows: Segment[] }) {
  const sum = (f: (s: Segment) => number) => rows.reduce((n, s) => n + f(s), 0)
  const needs = rows.filter((s) => attention(s)).length
  return (
    <Link to={`/discovery/segments?kind=${kind}`} className="rounded-lg border border-line p-3.5 transition-colors hover:border-accent/50 hover:bg-accent-soft/20">
      <div className="flex items-center gap-2.5">
        <span aria-hidden className={`grid size-8 place-items-center rounded-md ${tint}`}><Icon size={16} /></span>
        <div className="min-w-0">
          <div className="text-[12px] text-ink-3">{title}</div>
          <div className="text-[20px] leading-tight font-semibold text-ink tabular-nums">{formatCount(rows.length)}</div>
        </div>
      </div>
      <dl className="mt-3 grid grid-cols-3 gap-1 text-center">
        {([['Online', sum((s) => s.online)], ['Registered', sum((s) => s.registered)], ['Usable', sum((s) => s.usableSize)]] as const).map(([k, v]) => (
          <div key={k}><dt className="text-[11px] text-ink-3">{k}</dt><dd className="text-[13px] font-semibold text-ink tabular-nums">{formatCount(v)}</dd></div>
        ))}
      </dl>
      {needs > 0 && <p className="mt-2 flex items-center gap-1 text-[12px] text-warn"><AlertTriangle size={12} aria-hidden />{needs} need attention</p>}
    </Link>
  )
}

/** Every network by type, then the busiest ones right now. */
export function NetworksCard() {
  const q = useSegments()
  const all = q.data ?? []
  const busiest = [...all].filter((s) => s.online > 0 || s.openAlerts > 0)
    .sort((a, b) => b.openAlerts - a.openAlerts || b.online - a.online).slice(0, 6)
  return (
    <Card title="Networks" subtitle={q.data ? `${formatCount(all.length)} segments. Open one to see every address.` : undefined}
      action={<Link className="text-[12.5px] font-medium text-accent hover:underline" to="/discovery/segments">All segments</Link>}>
      {q.isPending ? <SkeletonRows rows={4} /> : q.isError ? <ErrorState error={q.error} onRetry={() => void q.refetch()} /> : all.length === 0
        ? <EmptyState title="No networks yet" body="Add DHCP, server and cloud networks in Network setup, or import them from your firewall's interface list." />
        : (
          <div className="space-y-4 p-4">
            <div className="grid gap-3 sm:grid-cols-3">
              {kinds.map((k) => <Tile key={k.kind} {...k} rows={all.filter((s) => s.kind === k.kind)} />)}
            </div>
            <div>
              <h3 className="mb-1.5 text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase">Most active now</h3>
              {busiest.length === 0 ? <p className="py-2 text-[13px] text-ink-3">Nothing is answering right now. Results appear after the next scan.</p> : (
                <ul className="divide-y divide-line rounded-md border border-line">
                  {busiest.map((s) => (
                    <li key={s.id}>
                      <Link to={`/segments/${s.id}`} className="flex items-center gap-3 px-3 py-2 hover:bg-surface-2">
                        <span className="min-w-0 flex-1">
                          <span className="block truncate text-[13px] font-medium text-ink">{s.name}</span>
                          <span className="block font-mono text-[11.5px] text-ink-3">{s.network}</span>
                        </span>
                        {s.openAlerts > 0 && <span className="flex items-center gap-1 text-[12px] text-warn"><AlertTriangle size={12} aria-hidden />{s.openAlerts}</span>}
                        <span className="w-20 text-right text-[12px] text-ink-2 tabular-nums">{formatCount(s.online)} online</span>
                      </Link>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </div>
        )}
    </Card>
  )
}
