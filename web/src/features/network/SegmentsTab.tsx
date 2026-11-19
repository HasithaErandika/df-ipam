import { useMutation } from '@tanstack/react-query'
import { ArrowRight, Pencil, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router'
import { Badge, Button, Card, EmptyState, EnvironmentBadge, ErrorState, FilterBar, FilterSelect, RowMenu, SearchInput, SkeletonRows, useConfirm } from '../../components/ui'
import { cx } from '../../components/ui/cx'
import { del } from '../../lib/api'
import { formatCount } from '../../lib/format'
import { label, segmentKinds } from '../../lib/labels'
import { useFilters } from '../../lib/listing'
import { inventoryKeys, useInvalidate, useSegments } from '../../lib/queries'
import type { Segment } from '../../lib/types'
import { ipv4NetworkInfo } from '../../lib/validation'
import { ScanButton } from '../shared/ScanButton'
import { SegmentForm } from './SegmentForm'
import { SegmentImport } from './SegmentImport'
import { attention, kindOrder, kindTitle, matches, notSwept, sorts } from './segmentList'

const chips = [
  { key: '', label: 'All' }, { key: 'dhcp', label: 'DHCP' }, { key: 'server', label: 'Server' }, { key: 'cloud', label: 'Cloud' }, { key: 'vpn', label: 'VPN' }, { key: 'attention', label: 'Needs attention' },
]

function SegmentCard({ s, onEdit, onDelete }: { s: Segment; onEdit: () => void; onDelete: () => void }) {
  const dhcp = s.kind === 'dhcp'
  const issue = attention(s)
  const network = ipv4NetworkInfo(s.network)

  return (
    <article className="flex min-w-0 flex-col rounded-md border border-line bg-surface p-3.5 transition-colors hover:border-line-strong">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <Link to={`/segments/${s.id}`} className="block truncate text-[14px] font-semibold text-ink hover:text-accent">{s.name}</Link>
        </div>
        <div className="flex shrink-0 items-start gap-1">
          <Badge tone={dhcp ? 'accent' : 'neutral'}>{label(segmentKinds, s.kind)}</Badge>
          {s.environmentName && <EnvironmentBadge name={s.environmentName} />}
          <RowMenu actions={[
            { label: 'Edit', icon: <Pencil size={14} aria-hidden />, onSelect: onEdit },
            { label: 'Delete', icon: <Trash2 size={14} aria-hidden />, danger: true, onSelect: onDelete },
          ]} />
        </div>
      </div>
      <div className="mt-3 rounded-md bg-surface-2 p-3">
        <div className="text-[10.5px] font-medium uppercase tracking-wide text-ink-3">Network</div>
        <div className="mt-0.5 truncate font-mono text-[15px] font-semibold text-ink">{s.network}</div>
        {network && <>
          <div className="mt-1 text-[11.5px] text-ink-2">Subnet mask <span className="font-mono">{network.mask}</span><span className="mx-1.5 text-ink-3">·</span>Broadcast <span className="font-mono">{network.broadcast}</span></div>
          <div className="mt-1 text-[11.5px] text-ink-2">Usable <span className="font-mono">{network.firstUsable} – {network.lastUsable}</span> <span className="text-ink-3">· {formatCount(network.totalAddresses)} total / {formatCount(network.usableAddresses)} usable</span></div>
        </>}
        <div className="mt-1 flex flex-wrap gap-x-2 text-[11.5px] text-ink-3">
          {(s.gateway || s.detectedGateway) && <span>Gateway <span className="font-mono text-ink-2">{s.gateway ?? s.detectedGateway}</span>{!s.gateway && ' (detected)'}</span>}
          {s.vlan !== null && <span>VLAN {s.vlan}</span>}
          {s.floor !== null && <span>Floor {s.floor}</span>}
        </div>
      </div>
      {issue && <p className="mt-2 truncate text-[11.5px] text-warn" title={issue}>{issue}</p>}
      <div className="mt-auto flex items-center justify-between gap-2 pt-3 text-[12px]">
        <span className="truncate text-ink-3">{s.kind === 'vpn' ? 'Shown from FortiGate VPN sessions' : s.kind === 'cloud' ? 'Not swept from the building' : 'Ready to scan'}</span>
        <div className="flex shrink-0 items-center gap-1.5">
          {!notSwept(s.kind) && <ScanButton segmentId={s.id} label="Scan" />}
          <Link to={`/segments/${s.id}`} className="inline-flex h-8 items-center gap-1 rounded-[5px] px-2 font-medium text-accent hover:bg-accent-soft">Open<ArrowRight size={13} aria-hidden /></Link>
        </div>
      </div>
    </article>
  )
}

export function SegmentsTab() {
  const q = useSegments()
  const f = useFilters(['show', 'q', 'sort'] as const)
  const [form, setForm] = useState<{ segment?: Segment } | null>(null)
  const confirm = useConfirm()
  const refresh = useInvalidate(...inventoryKeys)
  const remove = useMutation({ mutationFn: (s: Segment) => del(`/segments/${s.id}`), meta: { success: (s: Segment) => `${s.name} deleted.` }, onSuccess: () => void refresh() })
  const all = q.data ?? []
  const count = (key: string) => (key === '' ? all.length : key === 'attention' ? all.filter((s) => attention(s)).length : all.filter((s) => s.kind === key).length)
  const sort = sorts[f.values.sort] ?? sorts.name
  const shown = all.filter((s) => matches(s, f.values.q.trim()))
    .filter((s) => (f.values.show === 'attention' ? attention(s) : !f.values.show || s.kind === f.values.show))
    .sort(sort.compare)
  const groups = f.values.show ? [{ kind: '', rows: shown }] : kindOrder.map((k) => ({ kind: k, rows: shown.filter((s) => s.kind === k) })).filter((g) => g.rows.length)
  const ask = async (s: Segment) => {
    if (await confirm({ title: `Delete ${s.name}?`, body: 'Only an empty segment can be deleted: one with no register entries, reservations or requests. Its scan history is kept without a segment.', confirmLabel: 'Delete segment' })) remove.mutate(s)
  }

  return (
    <Card className="w-full min-w-0" title="Segments" subtitle="A visual map of DHCP, server and cloud networks. Open a segment to inspect addresses and discovery evidence."
      action={<div className="flex gap-2"><SegmentImport /><Button variant="primary" onClick={() => setForm({})}><Plus size={14} aria-hidden />Add segment</Button></div>}>
      {q.isPending ? <SkeletonRows rows={6} /> : q.isError ? <ErrorState error={q.error} onRetry={() => void q.refetch()} /> : all.length === 0 ? (
        <EmptyState title="No segments yet" body="Add one per floor or Wi-Fi (with its DHCP pool) and one per server or cloud network, or paste your firewall's interface list with Import from text." />
      ) : (
        <>
          <div className="flex flex-wrap gap-2 border-b border-line p-3">
            {chips.map((c) => {
              const active = f.values.show === c.key
              const n = count(c.key)
              return (
                <button key={c.key} type="button" aria-pressed={active} onClick={() => f.set('show', c.key)}
                  className={cx('inline-flex h-8 cursor-pointer items-center gap-2 rounded-full border px-3 text-[12.5px] font-medium transition-colors',
                    active ? 'border-accent bg-accent-soft text-accent' : 'border-line-strong bg-surface text-ink-2 hover:border-ink-3',
                    c.key === 'attention' && n > 0 && !active && 'text-warn')}>
                  {c.label}<span className={cx('rounded-full px-1.5 text-[11px] tabular-nums', active ? 'bg-accent text-white' : 'bg-surface-2 text-ink-3')}>{n}</span>
                </button>
              )
            })}
          </div>
          <FilterBar active={f.values.q ? 1 : 0} onClear={() => f.set('q', '')}>
            <SearchInput value={f.values.q} onChange={(v) => f.set('q', v)} placeholder="Search name, network, gateway, VLAN, environment" />
            <FilterSelect label="Sort" value={f.values.sort} onChange={(v) => f.set('sort', v)} anyLabel="Name"
              options={Object.entries(sorts).filter(([k]) => k !== 'name').map(([value, s]) => ({ value, text: s.text }))} />
          </FilterBar>
          {shown.length === 0 ? <EmptyState title="Nothing matches" body="Change the search or the filter." /> : (
            <div className="space-y-5 p-3 sm:p-4">
              {groups.map((g) => (
                <section key={g.kind || 'all'} aria-label={g.kind ? kindTitle[g.kind] : 'Matching segments'}>
                  {!f.values.show && g.kind && <h3 className="mb-2.5 flex items-center gap-2 text-[13px] font-semibold text-ink-2">{kindTitle[g.kind]}<Badge>{g.rows.length}</Badge></h3>}
                  <div className="grid min-w-0 grid-cols-1 gap-3 lg:grid-cols-2 2xl:grid-cols-4">
                    {g.rows.map((s) => <SegmentCard key={s.id} s={s} onEdit={() => setForm({ segment: s })} onDelete={() => void ask(s)} />)}
                  </div>
                </section>
              ))}
            </div>
          )}
        </>
      )}
      <SegmentForm open={form !== null} segment={form?.segment} onClose={() => setForm(null)} />
    </Card>
  )
}
