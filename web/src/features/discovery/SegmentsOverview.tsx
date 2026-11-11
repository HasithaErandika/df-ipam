import { AlertTriangle, ArrowRight } from 'lucide-react'
import { Link } from 'react-router'
import { Badge, Card, EmptyState, EnvironmentBadge, ErrorState, Meter, SearchInput, Segmented, SkeletonRows } from '../../components/ui'
import { formatAgo, formatCount, level, segmentCaption } from '../../lib/format'
import { label, segmentKinds } from '../../lib/labels'
import { useFilters } from '../../lib/listing'
import { useSegments } from '../../lib/queries'
import type { Segment } from '../../lib/types'
import { ScanButton } from '../shared/ScanButton'

const groups = [{ kind: 'dhcp', title: 'Floors' }, { kind: 'server', title: 'Server networks' }, { kind: 'cloud', title: 'Cloud networks' }] as const

function Figure({ label: text, value, tone, to }: { label: string; value: number; tone?: 'warn' | 'danger'; to?: string }) {
  const colour = value > 0 && tone === 'danger' ? 'text-danger' : value > 0 && tone === 'warn' ? 'text-warn' : 'text-ink'
  const body = <><dt className="text-[11px] text-ink-3">{text}</dt><dd className={`text-[15px] font-semibold tabular-nums ${colour}`}>{formatCount(value)}</dd></>
  return to && value > 0 ? <Link to={to} className="rounded-[4px] px-1 hover:bg-surface-2">{body}</Link> : <div className="px-1">{body}</div>
}

function SegmentCard({ s }: { s: Segment }) {
  const floor = s.kind === 'dhcp'
  const used = floor ? s.poolUsed : s.registered
  const of = floor ? s.poolSize : s.usableSize
  const pct = of ? Math.min(100, (used * 100) / of) : 0
  const findings = (kind: string) => `/discovery?segmentId=${s.id}&kind=${kind}`
  return (
    <article className="flex flex-col rounded-md border border-line bg-surface p-3.5 transition-colors hover:border-line-strong">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <Link to={`/segments/${s.id}`} className="block truncate text-[14px] font-semibold text-ink hover:text-accent">{s.name}</Link>
          <p className="truncate font-mono text-[11.5px] text-ink-3">{segmentCaption(s)}</p>
          {(s.gateway || s.detectedGateway) && (
            <p className="mt-0.5 flex items-center gap-1 text-[11.5px] text-ink-3">
              <span aria-hidden className="size-1.5 rounded-full bg-[var(--addr-gateway)]" />
              Gateway <span className="font-mono text-ink-2">{s.gateway ?? s.detectedGateway}</span>{!s.gateway && ' (detected)'}
            </p>
          )}
          {s.alertPrivateMacs && <p className="mt-0.5 text-[11.5px] text-ink-3">Alerts on private-MAC devices</p>}
        </div>
        {s.environmentName ? <EnvironmentBadge name={s.environmentName} /> : <Badge tone={floor ? 'accent' : 'neutral'}>{label(segmentKinds, s.kind)}</Badge>}
      </div>
      <div className="mt-3">
        <div className="mb-1 flex justify-between text-[12px] text-ink-3">
          <span>{floor ? 'DHCP pool in use' : 'Registered'}</span>
          <span className="tabular-nums">{of ? `${formatCount(used)} of ${formatCount(of)}` : floor ? 'No pool set' : '—'}</span>
        </div>
        <Meter wide pct={pct} level={level(pct, 75, 90)} label={`${s.name} use`} />
      </div>
      <dl className="mt-3 grid grid-cols-4 gap-1 border-t border-line pt-2.5 text-center">
        <Figure label="Answering" value={s.online} to={findings('online')} />
        <Figure label="Registered" value={s.registered} />
        <Figure label="Unknown" value={s.unknownDevices} tone="warn" to={findings('unknown')} />
        <Figure label="Alerts" value={s.openAlerts} tone="danger" to={`/alerts?segmentId=${s.id}`} />
      </dl>
      <div className="mt-auto flex items-center justify-between gap-2 pt-3 text-[12px]">
        <span className="text-ink-3">
          {s.kind === 'cloud' ? 'Not swept from the building' : s.lastSeen ? `Last answer ${formatAgo(s.lastSeen)}` : 'Nothing seen yet'}
        </span>
        <div className="flex items-center gap-1.5">
          {s.kind !== 'cloud' && <ScanButton segmentId={s.id} label="Scan" />}
          <Link to={`/segments/${s.id}`} className="inline-flex h-8 items-center gap-1 rounded-[5px] px-2 font-medium text-accent hover:bg-accent-soft">Open<ArrowRight size={13} aria-hidden /></Link>
        </div>
      </div>
    </article>
  )
}

/** Every segment at a glance, grouped by type, each linking to its addresses and findings. */
export function SegmentsOverview() {
  const q = useSegments()
  const f = useFilters(['kind', 'q'] as const)
  if (q.isPending) return <Card><SkeletonRows rows={6} /></Card>
  if (q.isError) return <Card><ErrorState error={q.error} onRetry={() => void q.refetch()} /></Card>
  const text = f.values.q.toLowerCase()
  const shown = q.data.filter((s) => (!f.values.kind || s.kind === f.values.kind)
    && (!text || s.name.toLowerCase().includes(text) || s.network.includes(text) || String(s.vlan ?? '').includes(text)))
  const alerts = q.data.reduce((n, s) => n + s.openAlerts, 0)
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <SearchInput value={f.values.q} onChange={(v) => f.set('q', v)} placeholder="Search name, network or VLAN" />
        <Segmented label="Segment type" value={f.values.kind} onChange={(v) => f.set('kind', v)}
          options={[{ value: '', text: `All ${q.data.length}` }, ...groups.map((g) => ({ value: g.kind, text: g.title.replace(' networks', '') }))]} />
        {alerts > 0 && <Link to="/alerts" className="ml-auto inline-flex items-center gap-1.5 text-[12.5px] font-medium text-danger hover:underline"><AlertTriangle size={14} aria-hidden />{alerts} open alerts across segments</Link>}
      </div>
      {q.data.length === 0 ? <Card><EmptyState title="No segments yet" body="Add each floor, server and cloud network in Settings > Segments, then scan them here." /></Card>
        : shown.length === 0 ? <Card><EmptyState title="No segments match" body="Change the search or the type." /></Card>
        : groups.map((g) => {
          const list = shown.filter((s) => s.kind === g.kind)
          if (list.length === 0) return null
          return (
            <section key={g.kind}>
              <h2 className="mb-2 text-[11px] font-semibold tracking-[.06em] text-ink-3 uppercase">{g.title} · {list.length}</h2>
              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">{list.map((s) => <SegmentCard key={s.id} s={s} />)}</div>
            </section>
          )
        })}
    </div>
  )
}
