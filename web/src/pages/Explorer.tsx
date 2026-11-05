import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, Grid3x3, List, Plus, Radar, Search } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useSearchParams } from 'react-router'
import { StartJobModal } from './Discovery'
import { allocationStates, DeviceIcon, deviceTypes, observationStates, OSBadge, StateBadges } from '../components/devices'
import { SavedFilters } from '../components/filters'
import { Badge, Button, Card, EmptyState, ErrorState, Field, FormError, Input, Modal, Mono, PageHeader, Pagination, Select, SkeletonRows, Td, Th } from '../components/ui'
import { Detail, SubnetPicker, Tabs, useDebounced } from '../components/workflow'
import { ApiError, get, getBody, list, patchPlain, post, queryString } from '../lib/api'
import { formatDateTime, humanAction } from '../lib/format'
import { useClampPage, usePaging } from '../lib/paging'
import { useCan } from '../lib/session'
import type { DeviceInfo, EvidenceItem, GridCell, HistoryEntry, IPRecord, Subnet } from '../lib/types'
import { RefSelect, RoutingDomainSelect } from '../components/SearchSelect'
import { useChecks } from '../components/workflow'
import { choose, ip, issues as check, optionalMAC } from '../lib/validation'

const heading = 'mb-1 mt-4 text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase first:mt-0'

function Section({ title, children }: { title: string; children: ReactNode }) {
  return <section><h3 className={heading}>{title}</h3>{children}</section>
}

function Source({ children }: { children: ReactNode }) {
  return <span className="ml-1 rounded-[3px] bg-surface-2 px-1 text-[11px] text-ink-3">{children}</span>
}

/** Everything known about the device behind the address, each part with its source. */
function DevicePanel({ id }: { id: string }) {
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['ip', id, 'device'], queryFn: () => get<DeviceInfo>(`/ip-addresses/${id}/device`) })
  if (isPending) return <SkeletonRows rows={4} />
  if (error) return <ErrorState error={error} onRetry={refetch} />
  const nothing = !data.device && !data.interface && !data.mac && data.dnsRecords.length === 0 && data.dhcpReservations.length === 0 && !data.hasEvidence
  return (
    <div>
      {nothing && <p className="rounded-[5px] bg-surface-2 px-3 py-2 text-[13px] text-ink-2">Nothing is known about a device here yet. Link a device record, or wait for an approved source to report it.</p>}
      {data.device && (
        <div className="flex items-start gap-3 rounded-[5px] border border-line p-3">
          <div className="grid size-10 shrink-0 place-items-center rounded-md bg-accent-soft"><DeviceIcon type={data.device.deviceType} size={20} /></div>
          <div className="min-w-0 flex-1">
            <div className="font-medium text-ink">{data.device.name} <Source>{data.device.sourceSystem}</Source></div>
            <div className="text-[12.5px] text-ink-2">{deviceTypes[data.device.deviceType]?.label ?? data.device.deviceType}{[data.device.vendor, data.device.model].filter(Boolean).length > 0 && ` · ${[data.device.vendor, data.device.model].filter(Boolean).join(' ')}`}</div>
            <dl className="mt-2">
              <Detail label="Operating system"><OSBadge family={data.device.osFamily === 'unknown' ? null : data.device.osFamily} version={data.device.osVersion || null} />{data.device.osVersion && data.device.osFamily !== 'unknown' && <span className="ml-1 text-ink-3">{data.device.osVersion}</span>}</Detail>
              {data.device.hostname && <Detail label="Hostname">{data.device.hostname}</Detail>}
              {data.device.serialNumber && <Detail label="Serial number"><Mono>{data.device.serialNumber}</Mono></Detail>}
              {data.device.managementIp && <Detail label="Management IP"><Mono>{data.device.managementIp}</Mono></Detail>}
              {data.device.siteName && <Detail label="Site">{data.device.siteName}</Detail>}
              {data.device.businessService && <Detail label="Business service">{data.device.businessService}</Detail>}
              <Detail label="Status">{data.device.status}{data.device.sensitivity !== 'standard' && <> · <Badge tone="warn">{data.device.sensitivity}</Badge></>}</Detail>
              {data.device.otherAddresses > 0 && <Detail label="Other addresses">{data.device.otherAddresses} more on this device</Detail>}
              {data.device.lastVerified && <Detail label="Last verified">{formatDateTime(data.device.lastVerified)}</Detail>}
            </dl>
          </div>
        </div>
      )}
      {(data.interface || data.mac) && (
        <dl className="mt-3">
          {data.interface && <Detail label="Interface">{data.interface.name}{data.interface.vlan && ` · VLAN ${data.interface.vlan}`}{data.interface.vdom && ` · ${data.interface.vdom}`} <Source>{data.interface.source}</Source></Detail>}
          {data.interface && <Detail label="Link">{data.interface.adminStatus} / {data.interface.operStatus}</Detail>}
          {data.mac && (
            <Detail label="MAC">
              <Mono>{data.mac.address}</Mono>
              {data.mac.locallyAdministered && <span className="ml-1"><Badge>Locally administered</Badge></span>}
              <div className="text-[12px] text-ink-3">{data.mac.locallyAdministered ? 'Often a randomised phone or laptop address, or a virtual NIC.' : `Vendor prefix ${data.mac.oui}`}</div>
            </Detail>
          )}
        </dl>
      )}
      {data.dnsRecords.length > 0 && (
        <Section title="DNS names">
          <ul>{data.dnsRecords.map((r) => <li key={`${r.recordType}${r.name}`} className="flex justify-between border-b border-line py-1 text-[13px] last:border-0"><span><Badge>{r.recordType}</Badge> {r.name}</span><span className="text-ink-3">{r.source ?? 'DNS'}{r.lastSeen && ` · ${formatDateTime(r.lastSeen)}`}</span></li>)}</ul>
        </Section>
      )}
      {data.dhcpReservations.length > 0 && (
        <Section title="DHCP reservations">
          <ul>{data.dhcpReservations.map((r, i) => <li key={i} className="flex justify-between border-b border-line py-1 text-[13px] last:border-0"><span>{r.hostname || '—'} {r.mac && <Mono>{r.mac}</Mono>}</span><span className="text-ink-3">{r.scope}</span></li>)}</ul>
        </Section>
      )}
      {data.sources.length > 0 && (
        <Section title="Seen by">
          <ul>{data.sources.map((s) => <li key={s.source} className="flex justify-between border-b border-line py-1 text-[13px] last:border-0"><span>{s.source} <span className="text-ink-3">· {s.reliability}% reliable</span></span><span className="text-ink-3">{s.observations} reports · last {formatDateTime(s.lastSeen)}</span></li>)}</ul>
        </Section>
      )}
      <p className="mt-3 text-[12px] text-ink-3">From inventory records and approved read-only sources only. DF-IPAM doesn't probe devices from this view, and shows an operating system only when a record states it.</p>
    </div>
  )
}

function EditRecord({ record, onClose }: { record: IPRecord; onClose: () => void }) {
  const queryClient = useQueryClient()
  const canAllocate = useCan('ip:allocate')
  const canReserve = useCan('ip:reserve')
  const canState = canAllocate || canReserve
  const [form, setForm] = useState({ hostname: record.hostname, fqdn: record.fqdn, mac: record.mac ?? '', notes: record.notes, deviceId: record.deviceId ?? '', allocationState: record.allocationState, reason: '' })
  const save = useMutation({
    mutationFn: () => patchPlain(`/ip-addresses/${record.id}`, {
      hostname: form.hostname, fqdn: form.fqdn, mac: form.mac, notes: form.notes, deviceId: form.deviceId,
      ...(form.allocationState !== record.allocationState ? { allocationState: form.allocationState, reason: form.reason } : {}),
    }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['ip'] }); queryClient.invalidateQueries({ queryKey: ['subnets'] }); onClose() },
  })
  const held = record.allocationState === 'allocated' || record.allocationState === 'reserved'
  const checks = useChecks(check({
    mac: optionalMAC(form.mac),
    reason: held && form.allocationState !== record.allocationState && !form.reason.trim() ? 'Give a reason for releasing this address.' : undefined,
  }))
  const errors: Record<string, string> = {}
  if (save.error instanceof ApiError) for (const d of save.error.details) errors[d.field] = d.issue
  Object.assign(errors, checks.shown)
  const set = (k: keyof typeof form) => (e: { target: { value: string } }) => setForm((f) => ({ ...f, [k]: e.target.value }))
  return (
    <Modal open onClose={onClose} title={`Edit ${record.address}`} subtitle="Changes are audited."
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => checks.submit(() => save.mutate())}>Save changes</Button></>}>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Hostname" htmlFor="ip-host"><Input id="ip-host" value={form.hostname} onChange={set('hostname')} /></Field>
        <Field label="FQDN" htmlFor="ip-fqdn"><Input id="ip-fqdn" value={form.fqdn} onChange={set('fqdn')} /></Field>
        <Field label="MAC" htmlFor="ip-mac" error={errors.mac}><Input id="ip-mac" className="font-mono" value={form.mac} onChange={set('mac')} /></Field>
        <Field label="Device" htmlFor="ip-device">
          <RefSelect<{ id: string; name: string; deviceType: string }> id="ip-device" value={form.deviceId} onChange={(v) => setForm((f) => ({ ...f, deviceId: v }))} path="/devices" emptyLabel="None"
            toOption={(d) => ({ value: d.id, label: d.name, detail: deviceTypes[d.deviceType]?.label ?? d.deviceType })} />
        </Field>
        <div className="col-span-2"><Field label="Notes" htmlFor="ip-notes"><Input id="ip-notes" value={form.notes} onChange={set('notes')} /></Field></div>
        {canState && (
          <>
            <Field label="Allocation state" htmlFor="ip-state" hint="Prefer the request workflow; this is a direct change.">
              <Select id="ip-state" value={form.allocationState} onChange={set('allocationState')}>
                {Object.entries(allocationStates).map(([k, v]) => <option key={k} value={k}>{v.label}</option>)}
              </Select>
            </Field>
            {form.allocationState !== record.allocationState && (
              <Field label="Reason" htmlFor="ip-reason" error={errors.reason}><Input id="ip-reason" value={form.reason} onChange={set('reason')} /></Field>
            )}
          </>
        )}
      </div>
      {Object.keys(errors).length === 0 && <div className="mt-3"><FormError error={save.error} /></div>}
    </Modal>
  )
}

function RecordModal({ id, onClose }: { id: string; onClose: () => void }) {
  const canEdit = useCan('inventory:edit')
  const [tab, setTab] = useState('device')
  const [editing, setEditing] = useState(false)
  const record = useQuery({ queryKey: ['ip', id], queryFn: () => get<IPRecord>(`/ip-addresses/${id}`) })
  const history = useQuery({ queryKey: ['ip', id, 'history'], enabled: tab === 'history', queryFn: () => get<HistoryEntry[]>(`/ip-addresses/${id}/history`) })
  const evidence = useQuery({ queryKey: ['ip', id, 'evidence'], enabled: tab === 'evidence', queryFn: () => get<EvidenceItem[]>(`/ip-addresses/${id}/evidence`) })
  const r = record.data
  return (
    <>
      <Modal open onClose={onClose} size="xl" title={r ? r.address : 'Address'}
        subtitle={r && <span className="flex flex-wrap items-center gap-2"><StateBadges allocation={r.allocationState} observation={r.observationState} /> {r.subnetName} <Mono>{r.subnetCidr}</Mono> · {r.routingDomainName}</span>}
        footer={r && canEdit ? <Button variant="primary" onClick={() => setEditing(true)}>Edit record</Button> : undefined}>
        {record.isPending && <SkeletonRows rows={6} />}
        {record.error && <ErrorState error={record.error} onRetry={record.refetch} />}
        {r && (
          <div className="grid gap-5 md:grid-cols-[1fr_1.2fr]">
            <section>
              <h3 className={heading}>Record</h3>
              <dl>
                <Detail label="Hostname">{r.hostname || '—'}</Detail>
                <Detail label="FQDN">{r.fqdn || '—'}</Detail>
                <Detail label="MAC">{r.mac ? <Mono>{r.mac}</Mono> : '—'}</Detail>
                <Detail label="Owner">{r.ownerEmail ?? '—'}</Detail>
                <Detail label="Application">{r.applicationName ?? '—'}</Detail>
                <Detail label="Business service">{r.businessServiceName ?? '—'}</Detail>
                <Detail label="Site">{r.siteName ?? '—'}</Detail>
                <Detail label="Confidence">{r.confidenceScore === null ? '—' : `${r.confidenceScore}%`}</Detail>
                {r.confidenceExplanation && <Detail label="Why">{r.confidenceExplanation}</Detail>}
                <Detail label="First seen">{r.firstSeen ? formatDateTime(r.firstSeen) : '—'}</Detail>
                <Detail label="Last seen">{r.lastSeen ? formatDateTime(r.lastSeen) : '—'}</Detail>
                <Detail label="Updated">{formatDateTime(r.updatedAt)}</Detail>
              </dl>
              {r.notes && <p className="mt-2 text-[13px] text-ink-2">{r.notes}</p>}
            </section>
            <section>
              <Tabs tabs={[{ key: 'device', label: 'Device' }, { key: 'evidence', label: 'Evidence' }, { key: 'history', label: 'History' }]} active={tab} onChange={setTab} />
              {tab === 'device' && <DevicePanel id={id} />}
              {tab === 'evidence' && (evidence.isPending ? <SkeletonRows rows={3} /> : evidence.error ? <ErrorState error={evidence.error} /> :
                evidence.data!.length === 0 ? <p className="text-[13px] text-ink-3">No raw evidence yet. Evidence stays separate from the record above.</p> : (
                  <ul>{evidence.data!.map((e, i) => <li key={i} className="flex justify-between border-b border-line py-1 text-[13px] last:border-0"><span>{e.source}{e.hostname && ` · ${e.hostname}`}{e.mac && <> · <Mono>{e.mac}</Mono></>}</span><span className="text-ink-3">{formatDateTime(e.observedAt)}</span></li>)}</ul>
                ))}
              {tab === 'history' && (history.isPending ? <SkeletonRows rows={3} /> : history.error ? <ErrorState error={history.error} /> :
                history.data!.length === 0 ? <p className="text-[13px] text-ink-3">No changes recorded.</p> : (
                  <ul>{history.data!.map((h, i) => <li key={i} className="flex justify-between border-b border-line py-1 text-[13px] last:border-0"><span>{humanAction(h.action)}</span><span className="text-ink-3">{h.actorEmail ?? 'System'} · {formatDateTime(h.occurredAt)}</span></li>)}</ul>
                ))}
            </section>
          </div>
        )}
      </Modal>
      {editing && r && <EditRecord record={r} onClose={() => setEditing(false)} />}
    </>
  )
}

function NewRecord({ onClose, onCreated }: { onClose: () => void; onCreated: (id: string) => void }) {
  const queryClient = useQueryClient()
  const [routingDomainId, setRd] = useState('')
  const [address, setAddress] = useState('')
  const [hostname, setHostname] = useState('')
  const create = useMutation({
    mutationFn: () => post<IPRecord>('/ip-addresses', { routingDomainId, address, hostname }),
    onSuccess: (r) => { queryClient.invalidateQueries({ queryKey: ['ip'] }); onClose(); onCreated(r.id) },
  })
  const checks = useChecks(check({ routingDomainId: choose(routingDomainId, 'a routing domain'), address: ip(address) }))
  const errors: Record<string, string> = {}
  if (create.error instanceof ApiError) for (const d of create.error.details) errors[d.field] = d.issue
  Object.assign(errors, checks.shown)
  return (
    <Modal open onClose={onClose} title="New IP record" subtitle="It joins the most specific subnet holding the address. Records are only kept for addresses of interest."
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={create.isPending} onClick={() => checks.submit(() => create.mutate())}>Create record</Button></>}>
      <div className="space-y-3.5">
        <Field label="Routing domain" htmlFor="nr-rd" error={errors.routingDomainId}>
          <RoutingDomainSelect id="nr-rd" value={routingDomainId} onChange={setRd} />
        </Field>
        <Field label="Address" htmlFor="nr-addr" error={errors.address}><Input id="nr-addr" className="font-mono" value={address} onChange={(e) => setAddress(e.target.value)} /></Field>
        <Field label="Hostname (optional)" htmlFor="nr-host"><Input id="nr-host" value={hostname} onChange={(e) => setHostname(e.target.value)} /></Field>
        {Object.keys(errors).length === 0 && <FormError error={create.error} />}
      </div>
    </Modal>
  )
}

const cellColour: Record<string, string> = {
  allocated: 'bg-[var(--state-allocated)] text-white', reserved: 'bg-[var(--state-reserved)] text-white',
  quarantined: 'bg-[var(--state-quarantined,#d6a400)] text-white', retired: 'bg-track text-ink-3',
  available_candidate: 'bg-ok-soft text-ok', candidate: 'bg-surface text-ink-3',
  network: 'bg-track text-ink-3', broadcast: 'bg-track text-ink-3', gateway: 'bg-accent-soft font-semibold text-accent',
  reserved_range: 'bg-warn-soft text-warn', dhcp: 'bg-[repeating-linear-gradient(135deg,var(--surface-2),var(--surface-2)_4px,var(--track)_4px,var(--track)_6px)] text-ink-2',
}

const gridLegend: [string, string][] = [['allocated', 'Allocated'], ['reserved', 'Reserved'], ['available_candidate', 'Recorded, available'], ['candidate', 'Available candidate'], ['gateway', 'Gateway'], ['reserved_range', 'Reserved range'], ['dhcp', 'DHCP pool']]

/** A page of the subnet's addresses. Large and IPv6 subnets load one range at a time. */
function SubnetGrid({ subnet, onOpen }: { subnet: Subnet; onOpen: (id: string) => void }) {
  const limit = 256
  const [offset, setOffset] = useState(0n)
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['ip', 'grid', subnet.id, offset.toString()], placeholderData: keepPreviousData,
    queryFn: () => getBody<{ data: GridCell[]; meta: { offset: string; limit: number; total: string } }>(`/subnets/${subnet.id}/addresses?offset=${offset}&limit=${limit}`),
  })
  const total = data ? BigInt(data.meta.total) : 0n
  return (
    <div className="p-3">
      <div className="mb-2 flex flex-wrap items-center justify-between gap-2 text-[12.5px] text-ink-2">
        <div className="flex flex-wrap gap-x-3 gap-y-1">{gridLegend.map(([k, label]) => <span key={k} className="flex items-center gap-1"><span className={`inline-block size-3 rounded-[2px] border border-line ${cellColour[k]}`} />{label}</span>)}</div>
        <div className="flex items-center gap-1">
          <span className="tabular-nums">{data && `${data.data[0]?.address ?? ''} – ${data.data[data.data.length - 1]?.address ?? ''}`}</span>
          <Button variant="ghost" className="h-7 px-1" aria-label="Previous range" disabled={offset === 0n} onClick={() => setOffset(offset - BigInt(limit) < 0n ? 0n : offset - BigInt(limit))}><ChevronLeft size={15} /></Button>
          <Button variant="ghost" className="h-7 px-1" aria-label="Next range" disabled={offset + BigInt(limit) >= total} onClick={() => setOffset(offset + BigInt(limit))}><ChevronRight size={15} /></Button>
        </div>
      </div>
      {isPending && <SkeletonRows rows={4} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data && (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(4.5rem,1fr))] gap-1">
          {data.data.map((cell) => {
            const key = cell.kind === 'record' ? cell.allocationState ?? 'candidate' : cell.kind
            const last = cell.address.split(/[.:]/).pop()
            const label = `${cell.address}: ${cell.kind === 'record' ? `${allocationStates[cell.allocationState ?? '']?.label ?? ''}, ${observationStates[cell.observationState ?? '']?.label ?? ''}${cell.hostname ? `, ${cell.hostname}` : ''}` : cell.kind.replace('_', ' ')}`
            return (
              <button key={cell.address} type="button" title={label} aria-label={label} disabled={!cell.id} onClick={() => cell.id && onOpen(cell.id)}
                className={`h-9 truncate rounded-[3px] border border-line px-1 text-left font-mono text-[11px] ${cellColour[key] ?? 'bg-surface'} ${cell.id ? 'cursor-pointer hover:ring-2 hover:ring-accent/40' : 'cursor-default'} ${cell.observationState === 'conflict_suspected' ? 'ring-2 ring-danger' : ''}`}>
                .{last}{cell.hostname && <span className="block truncate font-sans text-[10px] opacity-80">{cell.hostname}</span>}
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}

export function Explorer() {
  const canCreate = useCan('inventory:create')
  const canScan = useCan('discovery:run')
  const [scanning, setScanning] = useState(false)
  const [params, setParams] = useSearchParams()
  const paging = usePaging()
  const [creating, setCreating] = useState(false)
  const [subnet, setSubnet] = useState<Subnet | null>(null)
  const subnetId = params.get('subnetId') ?? ''
  const view = params.get('view') === 'grid' && subnetId ? 'grid' : 'table'
  const allocationState = params.get('allocationState') ?? ''
  const observationState = params.get('observationState') ?? ''
  const [search, setSearch] = useState(params.get('q') ?? '')
  const q = useDebounced(search.trim())
  const openId = params.get('id')
  const set = (updates: Record<string, string | null>) => setParams((prev) => {
    const next = new URLSearchParams(prev)
    for (const [k, v] of Object.entries(updates)) {
      if (v) next.set(k, v)
      else next.delete(k)
    }
    if (!('page' in updates) && !('id' in updates) && !('view' in updates)) next.delete('page')
    return next
  })
  const pinned = useQuery({ queryKey: ['subnet', subnetId], enabled: Boolean(subnetId) && subnet?.id !== subnetId, queryFn: () => get<Subnet>(`/subnets/${subnetId}`) })
  const current = subnet?.id === subnetId ? subnet : pinned.data ?? null
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, subnetId, allocationState, observationState, q })
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['ip', 'list', qs], placeholderData: keepPreviousData, enabled: view === 'table', queryFn: () => list<IPRecord>(`/ip-addresses${qs}`),
  })
  useClampPage(paging, data?.meta)
  const filtered = Boolean(subnetId || allocationState || observationState || q)
  return (
    <>
      <PageHeader title="IP Address Explorer" description="Every tracked address, the device behind it, and its history. Allocation and observation are always shown separately."
        actions={<>
          {canScan && <Button disabled={!current} title={current ? 'Discover this subnet under an approved policy' : 'Choose a subnet first'} onClick={() => setScanning(true)}><Radar size={15} aria-hidden /> Scan subnet</Button>}
          {canCreate && <Button variant="primary" onClick={() => setCreating(true)}><Plus size={15} aria-hidden /> New record</Button>}
        </>} />
      <Card>
        <div className="flex flex-wrap items-start gap-2 border-b border-line p-2.5">
          <div className="w-full max-w-md">
            <SubnetPicker id="ex-subnet" value={current} onChange={(s) => { setSubnet(s); set({ subnetId: s?.id ?? null, view: s ? view : null }) }} />
          </div>
          <div className="relative min-w-52 flex-1">
            <Search size={14} className="absolute top-1/2 left-2.5 -translate-y-1/2 text-ink-3" aria-hidden />
            <Input aria-label="Search addresses" placeholder="Address, hostname, FQDN or MAC" className="pl-8" value={search}
              onChange={(e) => { setSearch(e.target.value); paging.setPage(1) }} />
          </div>
          <Select aria-label="Allocation state" className="w-44!" value={allocationState} onChange={(e) => set({ allocationState: e.target.value })}>
            <option value="">Any allocation</option>
            {Object.entries(allocationStates).map(([k, v]) => <option key={k} value={k}>{v.label}</option>)}
          </Select>
          <Select aria-label="Observation state" className="w-44!" value={observationState} onChange={(e) => set({ observationState: e.target.value })}>
            <option value="">Any observation</option>
            {Object.entries(observationStates).map(([k, v]) => <option key={k} value={k}>{v.label}</option>)}
          </Select>
          <SavedFilters page="ip-addresses" current={{ subnetId, allocationState, observationState, q }}
            onApply={(f) => { setSearch(f.q ?? ''); set({ subnetId: f.subnetId ?? null, allocationState: f.allocationState ?? null, observationState: f.observationState ?? null }) }} />
          {subnetId && (
            <div className="ml-auto flex gap-1" role="group" aria-label="View">
              <Button variant={view === 'table' ? 'primary' : 'secondary'} aria-pressed={view === 'table'} onClick={() => set({ view: null })}><List size={14} aria-hidden /> Table</Button>
              <Button variant={view === 'grid' ? 'primary' : 'secondary'} aria-pressed={view === 'grid'} onClick={() => set({ view: 'grid' })}><Grid3x3 size={14} aria-hidden /> Grid</Button>
            </div>
          )}
        </div>
        {view === 'grid' && current && <SubnetGrid key={current.id} subnet={current} onOpen={(id) => set({ id })} />}
        {view === 'table' && (
          <>
            {isPending && <SkeletonRows rows={8} />}
            {error && <ErrorState error={error} onRetry={refetch} />}
            {data?.data.length === 0 && (filtered
              ? <EmptyState title="No addresses match" body={subnetId ? 'Only addresses of interest have records. Switch to the grid to see every address in the subnet.' : undefined} />
              : <EmptyState title="No IP records yet" body="Records appear as addresses are allocated, imported or reported by approved sources." />)}
            {data && data.data.length > 0 && (
              <>
                <div className="overflow-x-auto">
                  <table className="w-full border-collapse">
                    <thead><tr><Th>Address</Th><Th>Device</Th><Th>OS</Th><Th>MAC</Th><Th>Subnet</Th><Th>Owner</Th><Th>State</Th><Th>Last seen</Th></tr></thead>
                    <tbody>
                      {data.data.map((r) => (
                        <tr key={r.id} className="cursor-pointer hover:bg-surface-2" onClick={() => set({ id: r.id })}>
                          <Td><button type="button" className="cursor-pointer hover:text-accent" onClick={() => set({ id: r.id })}><Mono>{r.address}</Mono></button></Td>
                          <Td>
                            <span className="flex items-center gap-2">
                              <DeviceIcon type={r.deviceType} />
                              <span className="min-w-0">
                                <span className="block truncate text-ink">{r.deviceName || r.hostname || '—'}</span>
                                {r.deviceType && <span className="block text-[11.5px] text-ink-3">{deviceTypes[r.deviceType]?.label}{r.vendor && ` · ${r.vendor}`}</span>}
                              </span>
                            </span>
                          </Td>
                          <Td><OSBadge family={r.osFamily} version={r.operatingSystem} /></Td>
                          <Td>{r.mac ? <Mono>{r.mac}</Mono> : <span className="text-ink-3">—</span>}</Td>
                          <Td className="whitespace-nowrap text-ink-2">{r.subnetName} <span className="text-ink-3">· {r.routingDomainName}</span></Td>
                          <Td className="text-ink-2">{r.ownerEmail ?? '—'}</Td>
                          <Td><StateBadges allocation={r.allocationState} observation={r.observationState} /></Td>
                          <Td className="whitespace-nowrap text-ink-2">{r.lastSeen ? formatDateTime(r.lastSeen) : '—'}</Td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
              </>
            )}
          </>
        )}
      </Card>
      {openId && <RecordModal key={openId} id={openId} onClose={() => set({ id: null })} />}
      {scanning && current && <StartJobModal subnet={current} onClose={() => setScanning(false)} />}
      {creating && <NewRecord onClose={() => setCreating(false)} onCreated={(id) => set({ id })} />}
    </>
  )
}
