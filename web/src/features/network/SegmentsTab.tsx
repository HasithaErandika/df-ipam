import { useMutation } from '@tanstack/react-query'
import { AlertTriangle, CloudOff, ListTree, Pencil, Plus, ScanSearch, Smartphone, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router'
import { Badge, Button, Card, EmptyState, EnvironmentBadge, ErrorState, FilterBar, FilterSelect, RowMenu, SearchInput, SkeletonRows, useConfirm } from '../../components/ui'
import { cx } from '../../components/ui/cx'
import { del } from '../../lib/api'
import { formatAgo } from '../../lib/format'
import { useFilters } from '../../lib/listing'
import { inventoryKeys, useInvalidate, useSegments } from '../../lib/queries'
import type { Segment } from '../../lib/types'
import { SegmentForm } from './SegmentForm'
import { SegmentImport } from './SegmentImport'
import { attention, kindOrder, kindTitle, matches, sorts, use } from './segmentList'

const chips = [
  { key: '', label: 'All' }, { key: 'dhcp', label: 'Floors' }, { key: 'server', label: 'Server' }, { key: 'cloud', label: 'Cloud' }, { key: 'attention', label: 'Needs attention' },
]

function UseBar({ s }: { s: Segment }) {
  const u = use(s)
  const tone = u.pct >= 90 ? 'bg-danger' : u.pct >= 75 ? 'bg-[var(--meter-warning)]' : 'bg-accent'
  return (
    <div className="w-36" title={u.text}>
      <div className="h-1.5 overflow-hidden rounded-full bg-track"><div className={cx('h-full rounded-full', tone)} style={{ width: `${Math.min(u.pct, 100)}%` }} /></div>
      <div className="mt-1 truncate text-[11.5px] text-ink-3">{u.text}</div>
    </div>
  )
}

function Flags({ s }: { s: Segment }) {
  return (
    <span className="flex items-center gap-1.5 text-ink-3">
      {s.kind === 'cloud' && <span title="Never swept from the building"><CloudOff size={14} aria-label="Not swept" /></span>}
      {s.checkPorts && <span title="Checks common ports"><ScanSearch size={14} aria-label="Port checks on" /></span>}
      {s.alertPrivateMacs && <span title="Alerts on private-MAC devices"><Smartphone size={14} aria-label="Private-MAC alerts on" /></span>}
    </span>
  )
}

function Row({ s, onEdit, onDelete }: { s: Segment; onEdit: () => void; onDelete: () => void }) {
  const issue = attention(s)
  return (
    <tr className="group border-b border-line last:border-0 hover:bg-surface-2/70">
      <td className="px-3 py-2.5 align-top">
        <Link className="font-medium text-ink hover:text-accent hover:underline" to={`/segments/${s.id}`}>{s.name}</Link>
        {s.description && <div className="max-w-[28rem] truncate text-[12px] text-ink-3" title={s.description}>{s.description}</div>}
        {issue && <div className="mt-0.5 flex items-center gap-1 text-[12px] text-warn"><AlertTriangle size={12} aria-hidden />{issue}</div>}
      </td>
      <td className="px-3 py-2.5 align-top">
        <div className="font-mono text-[12.5px] text-ink">{s.network}</div>
        <div className="font-mono text-[11.5px] text-ink-3">
          {[s.gateway ? `gw ${s.gateway}` : s.detectedGateway ? `gw ${s.detectedGateway} (seen)` : null, s.vlan !== null ? `VLAN ${s.vlan}` : null, s.floor !== null ? `Floor ${s.floor}` : null].filter(Boolean).join(' · ') || '—'}
        </div>
      </td>
      <td className="px-3 py-2.5 align-top max-md:hidden">
        {s.kind === 'dhcp'
          ? (s.dhcpStart ? <span className="font-mono text-[12px] text-ink-2">{s.dhcpStart} – {s.dhcpEnd}</span> : <span className="text-[12px] text-warn">No pool</span>)
          : <EnvironmentBadge name={s.environmentName} />}
      </td>
      <td className="px-3 py-2.5 align-top max-lg:hidden"><UseBar s={s} /></td>
      <td className="px-3 py-2.5 align-top text-[12px] text-ink-2 max-xl:hidden">
        <div className="tabular-nums">{s.online} online</div>
        <div className="text-ink-3">{s.lastSeen ? `seen ${formatAgo(s.lastSeen)}` : 'nothing seen yet'}</div>
      </td>
      <td className="px-3 py-2.5 align-top max-md:hidden"><Flags s={s} /></td>
      <td className="px-3 py-2.5 text-right align-top whitespace-nowrap">
        <span className="inline-flex items-center gap-1">
          <Link to={`/segments/${s.id}`} className="inline-flex h-7 items-center gap-1 rounded-[5px] border border-line-strong px-2 text-[12px] font-medium text-ink hover:bg-surface-2"><ListTree size={13} aria-hidden />Addresses</Link>
          <RowMenu actions={[
            { label: 'Edit', icon: <Pencil size={14} aria-hidden />, onSelect: onEdit },
            { label: 'Delete', icon: <Trash2 size={14} aria-hidden />, danger: true, onSelect: onDelete },
          ]} />
        </span>
      </td>
    </tr>
  )
}

const th = 'border-b border-line bg-surface-2 px-3 py-2 text-left text-[11px] font-semibold tracking-[.04em] whitespace-nowrap text-ink-3 uppercase'

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
    <Card title="Segments" subtitle="Floors, server networks and cloud networks. Networks never overlap; open one to see every address."
      action={<div className="flex gap-2"><SegmentImport /><Button variant="primary" onClick={() => setForm({})}><Plus size={14} aria-hidden />Add segment</Button></div>}>
      {q.isPending ? <SkeletonRows rows={6} /> : q.isError ? <ErrorState error={q.error} onRetry={() => void q.refetch()} /> : all.length === 0 ? (
        <EmptyState title="No segments yet" body="Add one per floor (with its DHCP pool) and one per server or cloud network, or paste your firewall's interface list with Import from text." />
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
            <div className="overflow-x-auto">
              <table className="w-full border-collapse text-[13px]">
                <thead><tr>
                  <th className={th}>Segment</th><th className={th}>Network</th><th className={cx(th, 'max-md:hidden')}>Pool / environment</th>
                  <th className={cx(th, 'max-lg:hidden')}>Use</th><th className={cx(th, 'max-xl:hidden')}>Activity</th><th className={cx(th, 'max-md:hidden')}>Discovery</th><th className={th} />
                </tr></thead>
                {groups.map((g) => (
                  <tbody key={g.kind || 'all'}>
                    {g.kind && (
                      <tr><td colSpan={7} className="border-b border-line bg-bg px-3 py-1.5 text-[12px] font-semibold text-ink-2">
                        {kindTitle[g.kind]} <Badge>{g.rows.length}</Badge>
                      </td></tr>
                    )}
                    {g.rows.map((s) => <Row key={s.id} s={s} onEdit={() => setForm({ segment: s })} onDelete={() => void ask(s)} />)}
                  </tbody>
                ))}
              </table>
            </div>
          )}
        </>
      )}
      <SegmentForm open={form !== null} segment={form?.segment} onClose={() => setForm(null)} />
    </Card>
  )
}
