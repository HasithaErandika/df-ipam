import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Trash2 } from 'lucide-react'
import { useState, type FormEvent } from 'react'
import { AddressDetail, NewAddress } from '../../components/AddressDetail'
import { AddressGrid } from '../../components/AddressGrid'
import { Badge, Button, ConfirmDialog, ErrorState, FormError, Input, Modal, Mono, SkeletonRows } from '../../components/ui'
import { del, get, list, post } from '../../lib/api'
import { environments, formatCapacity, formatDateTime } from '../../lib/format'
import { useCan } from '../../lib/session'
import type { FreeRanges, Subnet, Utilisation } from '../../lib/types'
import { Calculated, Detail, sensitivities, SubnetForm } from './SubnetForm'

const heading = 'mb-1 text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase'
const tabs = { overview: 'Overview', addresses: 'Addresses', ranges: 'Reserved ranges and DHCP' } as const
type Tab = keyof typeof tabs

interface RangeRow { id: string; rangeStart: string; rangeEnd: string; reason?: string; provider?: string; name?: string }

function invalidate(qc: ReturnType<typeof useQueryClient>, id: string) {
  qc.invalidateQueries({ queryKey: ['subnets'] })
  qc.invalidateQueries({ queryKey: ['ranges', id] })
  qc.invalidateQueries({ queryKey: ['summary'] })
}

/** Reserved ranges or DHCP scopes for one subnet, with add and remove. */
function RangeList({ subnet, kind }: { subnet: Subnet; kind: 'reserved' | 'dhcp' }) {
  const qc = useQueryClient()
  const canCreate = useCan('inventory:create')
  const canDelete = useCan('inventory:delete')
  const path = kind === 'reserved' ? '/reserved-ranges' : '/dhcp-scopes'
  const rows = useQuery({ queryKey: ['ranges', subnet.id, kind], queryFn: () => list<RangeRow>(`${path}?subnetId=${subnet.id}&pageSize=200`) })
  const [start, setStart] = useState('')
  const [end, setEnd] = useState('')
  const [label, setLabel] = useState('')
  const add = useMutation({
    mutationFn: () => post(path, kind === 'reserved'
      ? { subnetId: subnet.id, rangeStart: start, rangeEnd: end || start, reason: label, provider: 'manual' }
      : { subnetId: subnet.id, rangeStart: start, rangeEnd: end || start, name: label }),
    onSuccess: () => { invalidate(qc, subnet.id); setStart(''); setEnd(''); setLabel('') },
  })
  const remove = useMutation({ mutationFn: (id: string) => del(`${path}/${id}`), onSuccess: () => invalidate(qc, subnet.id) })
  const submit = (e: FormEvent) => { e.preventDefault(); add.mutate() }
  const title = kind === 'reserved' ? 'Reserved ranges' : 'DHCP scopes'
  return (
    <section>
      <h3 className={heading}>{title}</h3>
      <p className="mb-2 text-[13px] text-ink-3">{kind === 'reserved'
        ? 'Counted in utilisation and never offered as candidates. Cloud provider ranges are added automatically.'
        : 'Pools served by DHCP. They block candidates but are not counted as utilisation.'}</p>
      {rows.isPending && <SkeletonRows rows={2} />}
      {rows.error && <ErrorState error={rows.error} onRetry={rows.refetch} />}
      {rows.data?.data.length === 0 && <p className="text-[13px] text-ink-3">None.</p>}
      <ul>
        {rows.data?.data.map((r) => (
          <li key={r.id} className="flex items-center justify-between gap-2 border-b border-line py-1 last:border-0">
            <span><Mono>{r.rangeStart === r.rangeEnd ? r.rangeStart : `${r.rangeStart} – ${r.rangeEnd}`}</Mono> <span className="text-[13px] text-ink-2">{r.reason || r.name}</span> {r.provider && r.provider !== 'manual' && <Badge tone="accent">{r.provider.toUpperCase()}</Badge>}</span>
            {canDelete && !subnet.archived && <Button variant="ghost" className="h-7 px-1.5 hover:text-danger" aria-label={`Remove ${r.rangeStart}`} busy={remove.isPending && remove.variables === r.id} onClick={() => remove.mutate(r.id)}><Trash2 size={14} /></Button>}
          </li>
        ))}
      </ul>
      {remove.error && <div className="mt-2"><FormError error={remove.error} /></div>}
      {canCreate && !subnet.archived && (
        <form onSubmit={submit} className="mt-2 grid grid-cols-[1fr_1fr_1fr_auto] gap-2">
          <Input aria-label={`${title}: first address`} placeholder="First" className="font-mono" required value={start} onChange={(e) => setStart(e.target.value)} />
          <Input aria-label={`${title}: last address`} placeholder="Last" className="font-mono" value={end} onChange={(e) => setEnd(e.target.value)} />
          <Input aria-label={`${title}: ${kind === 'reserved' ? 'reason' : 'name'}`} placeholder={kind === 'reserved' ? 'Reason' : 'Name'} value={label} onChange={(e) => setLabel(e.target.value)} />
          <Button type="submit" busy={add.isPending}>Add</Button>
          {add.error && <div className="col-span-4"><FormError error={add.error} /></div>}
        </form>
      )}
    </section>
  )
}

export function SubnetDetail({ id, onClose }: { id: string | null; onClose: () => void }) {
  const qc = useQueryClient()
  const canEdit = useCan('inventory:edit')
  const canDelete = useCan('inventory:delete')
  const canCreate = useCan('inventory:create')
  const [tab, setTab] = useState<Tab>('overview')
  const [confirming, setConfirming] = useState<'archive' | 'delete' | null>(null)
  const [editing, setEditing] = useState(false)
  const [record, setRecord] = useState<string | null>(null)
  const [candidate, setCandidate] = useState<string | null>(null)
  const subnet = useQuery({ queryKey: ['subnets', 'one', id], enabled: id !== null, queryFn: () => get<Subnet>(`/subnets/${id}`) })
  const usage = useQuery({ queryKey: ['subnets', id, 'utilisation'], enabled: id !== null, queryFn: () => get<Utilisation>(`/subnets/${id}/utilisation`) })
  const free = useQuery({ queryKey: ['subnets', id, 'free'], enabled: id !== null, queryFn: () => get<FreeRanges>(`/subnets/${id}/free-ranges`) })
  const act = useMutation({
    mutationFn: (kind: 'archive' | 'delete') => kind === 'archive' ? post<void>(`/subnets/${id}/archive`) : del(`/subnets/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['subnets'] })
      qc.invalidateQueries({ queryKey: ['summary'] })
      setConfirming(null)
      onClose()
    },
  })
  if (id === null) return null
  const s = subnet.data
  const footer = s && !s.archived && (canEdit || canDelete) ? (
    <>
      {canDelete && <Button variant="ghost" className="mr-auto text-danger!" onClick={() => { act.reset(); setConfirming('delete') }}>Delete</Button>}
      {canEdit && <Button onClick={() => { act.reset(); setConfirming('archive') }}>Archive</Button>}
      {canEdit && <Button variant="primary" onClick={() => setEditing(true)}>Edit subnet</Button>}
    </>
  ) : undefined
  return (
    <>
      <Modal open size="xl" onClose={onClose} title={s?.name ?? 'Subnet'} subtitle={s ? <span><Mono>{s.calculated.cidr}</Mono> {s.archived && <Badge>Archived</Badge>} {s.hasOverlapException && <Badge tone="warn">Overlap exception</Badge>}</span> : undefined} footer={footer}>
        {subnet.isPending && <SkeletonRows rows={6} />}
        {subnet.error && <ErrorState error={subnet.error} onRetry={subnet.refetch} />}
        {s && (
          <>
            <div role="tablist" className="mb-4 flex gap-1 border-b border-line">
              {(Object.keys(tabs) as Tab[]).map((t) => (
                <button key={t} role="tab" type="button" aria-selected={tab === t} onClick={() => setTab(t)}
                  className={`-mb-px cursor-pointer border-b-2 px-3 py-1.5 text-[13px] ${tab === t ? 'border-accent font-medium text-ink' : 'border-transparent text-ink-2 hover:text-ink'}`}>{tabs[t]}</button>
              ))}
            </div>
            {tab === 'overview' && (
              <div className="grid gap-5 md:grid-cols-2">
                <section>
                  <h3 className={heading}>Capacity</h3>
                  {usage.isPending && <SkeletonRows rows={5} />}
                  {usage.error && <ErrorState error={usage.error} />}
                  {usage.data && (
                    <dl>
                      <Detail label="Utilisation"><span className="tabular-nums">{usage.data.utilisationPct}%</span></Detail>
                      <Detail label="Allocated"><span className="tabular-nums">{usage.data.allocated}</span></Detail>
                      <Detail label="Reserved"><span className="tabular-nums">{usage.data.reserved}</span></Detail>
                      <Detail label="Quarantined"><span className="tabular-nums">{usage.data.quarantined}</span></Detail>
                      <Detail label="Reserved ranges"><span className="tabular-nums">{usage.data.reservedRangeAddresses}</span></Detail>
                      <Detail label="DHCP pool"><span className="tabular-nums">{usage.data.dhcpPoolAddresses}</span></Detail>
                      <Detail label="Available candidates"><span className="tabular-nums">{formatCapacity(usage.data.availableCandidates, s.calculated.family, s.calculated.prefixLength)}</span></Detail>
                    </dl>
                  )}
                  <h3 className={`${heading} mt-4`}>Addressing</h3>
                  <Calculated info={s.calculated} />
                </section>
                <section>
                  <h3 className={heading}>Mapping</h3>
                  <dl>
                    <Detail label="Routing domain">{s.routingDomainName}</Detail>
                    <Detail label="Environment">{environments[s.environment]}</Detail>
                    <Detail label="Sensitivity">{sensitivities[s.sensitivity] ?? s.sensitivity}</Detail>
                    <Detail label="Network block">{s.networkBlock?.name ?? 'Not set'}</Detail>
                    <Detail label="Site">{s.site?.name ?? 'Not set'}</Detail>
                    <Detail label="Network zone">{s.networkZone?.name ?? 'Not set'}</Detail>
                    <Detail label="VLAN">{s.vlan?.name ?? 'Not set'}</Detail>
                    <Detail label="VDOM">{s.vdom?.name ?? 'Not set'}</Detail>
                    <Detail label="Business unit">{s.businessUnit?.name ?? 'Not set'}</Detail>
                    <Detail label="Business service">{s.businessService?.name ?? 'Not set'}</Detail>
                    <Detail label="Technical owner">{s.technicalOwner?.name ?? 'Not set'}</Detail>
                    <Detail label="Business owner">{s.businessOwner?.name ?? 'Not set'}</Detail>
                    <Detail label="Gateway">{s.gateway ? <Mono>{s.gateway}</Mono> : 'Not set'}</Detail>
                    <Detail label="Security zone">{s.securityZone || 'Not set'}</Detail>
                    <Detail label="Discovery">{s.discoveryEnabled ? 'Included' : 'Not included'}</Detail>
                    <Detail label="Thresholds">{s.warnThreshold}% warning, {s.criticalThreshold}% critical</Detail>
                    <Detail label="Updated">{formatDateTime(s.updatedAt)}</Detail>
                  </dl>
                  <h3 className={`${heading} mt-4`}>Free ranges</h3>
                  {free.data?.ranges.length === 0 && <p className="text-[13px] text-ink-3">No free ranges.</p>}
                  {free.data && free.data.ranges.length > 0 && (
                    <ul className="max-h-40 overflow-y-auto">
                      {free.data.ranges.map((r) => (
                        <li key={r.first} className="flex justify-between gap-3 border-b border-line py-1 last:border-0">
                          <Mono>{r.first === r.last ? r.first : `${r.first} – ${r.last}`}</Mono>
                          <span className="text-[12.5px] text-ink-3">{r.cidrs.slice(0, 2).join(', ')}{r.cidrs.length > 2 ? '…' : ''}</span>
                        </li>
                      ))}
                    </ul>
                  )}
                  {free.data?.truncated && <p className="mt-1 text-[12px] text-ink-3">Showing the first {free.data.ranges.length} of {free.data.rangeCount}.</p>}
                </section>
                {s.description && <p className="text-ink-2 md:col-span-2">{s.description}</p>}
              </div>
            )}
            {tab === 'addresses' && (
              <AddressGrid subnetId={s.id} info={s.calculated} onRecord={setRecord}
                onCandidate={canCreate && !s.archived ? setCandidate : undefined} />
            )}
            {tab === 'ranges' && (
              <div className="grid gap-6 md:grid-cols-2">
                <RangeList subnet={s} kind="reserved" />
                <RangeList subnet={s} kind="dhcp" />
              </div>
            )}
          </>
        )}
      </Modal>
      {s && editing && <SubnetForm subnet={s} onClose={() => setEditing(false)} />}
      <AddressDetail id={record} onClose={() => setRecord(null)} />
      {s && <NewAddress open={candidate !== null} onClose={() => setCandidate(null)} routingDomainId={s.routingDomainId} address={candidate ?? ''} />}
      <ConfirmDialog
        open={confirming !== null}
        title={confirming === 'delete' ? 'Delete this subnet?' : 'Archive this subnet?'}
        body={confirming === 'delete'
          ? `${s?.calculated.cidr ?? ''} is removed for good. Subnets with IP records or other linked data can only be archived.`
          : `${s?.calculated.cidr ?? ''} leaves the active inventory, and its range can be used again. History is kept.`}
        confirmLabel={confirming === 'delete' ? 'Delete' : 'Archive'}
        busy={act.isPending}
        error={act.error}
        onConfirm={() => confirming && act.mutate(confirming)}
        onCancel={() => setConfirming(null)}
      />
    </>
  )
}
