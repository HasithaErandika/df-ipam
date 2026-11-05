import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, Grid3x3, Plus, Search, SlidersHorizontal, TableProperties } from 'lucide-react'
import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { AddressDetail, NewAddress } from '../components/AddressDetail'
import { AddressGrid } from '../components/AddressGrid'
import { allocationStates, ColumnMenu, observationStates, SavedFilters, SelectionBar, StateBadges } from '../components/inventory'
import { Button, Card, EmptyState, ErrorState, Field, FormError, Input, Modal, Mono, PageHeader, Pagination, Select, SkeletonRows, Td, Th } from '../components/ui'
import { get as getOne, list, post, queryString } from '../lib/api'
import { formatDateTime } from '../lib/format'
import { optionLabel, useColumns, useDebounced, useOptions, useUrlFilters } from '../lib/hooks'
import { useClampPage, usePaging } from '../lib/paging'
import { useCan } from '../lib/session'
import type { IpAddress, Subnet } from '../lib/types'
import { useRoutingDomains } from './subnets/SubnetForm'

const filterKeys = ['q', 'routingDomainId', 'subnetId', 'allocationState', 'observationState', 'family', 'siteId', 'vlanId', 'vdomId', 'ownerId', 'businessServiceId', 'sort'] as const
const moreKeys = ['siteId', 'vlanId', 'vdomId', 'ownerId', 'businessServiceId', 'family']

const columns = {
  hostname: 'Hostname', state: 'State', subnet: 'Subnet', routingDomain: 'Routing domain', mac: 'MAC', owner: 'Owner',
  service: 'Business service', device: 'Device', lastSeen: 'Last seen', updated: 'Updated',
}

const keep = '__keep'
const clear = '__clear'

function BulkEdit({ ids, onClose, onDone }: { ids: string[]; onClose: () => void; onDone: () => void }) {
  const qc = useQueryClient()
  const mayAllocate = useCan('ip:allocate')
  const mayReserve = useCan('ip:reserve')
  const owners = useOptions('/users/lookup')
  const services = useOptions('/business-services')
  const apps = useOptions('/applications')
  const [v, setV] = useState({ ownerId: keep, businessServiceId: keep, applicationId: keep, allocationState: keep, reason: '' })
  const set = (k: keyof typeof v) => (e: { target: { value: string } }) => setV({ ...v, [k]: e.target.value })
  const changes: Record<string, string> = {}
  for (const k of ['ownerId', 'businessServiceId', 'applicationId'] as const) if (v[k] !== keep) changes[k] = v[k] === clear ? '' : v[k]
  if (v.allocationState !== keep) Object.assign(changes, { allocationState: v.allocationState, reason: v.reason })
  const save = useMutation({
    mutationFn: () => post('/ip-addresses/bulk', { ids, changes }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['ip-addresses'] })
      qc.invalidateQueries({ queryKey: ['subnets'] })
      qc.invalidateQueries({ queryKey: ['summary'] })
      onDone()
      onClose()
    },
  })
  const submit = (e: FormEvent) => { e.preventDefault(); save.mutate() }
  const choose = (k: 'ownerId' | 'businessServiceId' | 'applicationId', label: string, q: ReturnType<typeof useOptions>) => (
    <Field label={label} htmlFor={`bulk-ip-${k}`}>
      <Select id={`bulk-ip-${k}`} value={v[k]} onChange={set(k)}>
        <option value={keep}>Keep as is</option><option value={clear}>Clear</option>
        {q.data?.map((o) => <option key={o.id} value={o.id}>{optionLabel(o)}</option>)}
      </Select>
    </Field>
  )
  return (
    <Modal open onClose={onClose} size="lg" title={`Edit ${ids.length} record${ids.length === 1 ? '' : 's'}`}
      subtitle="Only the fields you change are applied. If any record can't take the change, none are changed."
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" type="submit" form="bulk-ip" disabled={Object.keys(changes).length === 0} busy={save.isPending}>Apply to {ids.length}</Button></>}>
      <form id="bulk-ip" onSubmit={submit} className="grid gap-3 sm:grid-cols-2">
        {choose('ownerId', 'Owner', owners)}
        {choose('businessServiceId', 'Business service', services)}
        {choose('applicationId', 'Application', apps)}
        {(mayAllocate || mayReserve) && (
          <Field label="Allocation state" htmlFor="bulk-ip-state">
            <Select id="bulk-ip-state" value={v.allocationState} onChange={set('allocationState')}>
              <option value={keep}>Keep as is</option>
              {Object.entries(allocationStates).map(([k, label]) => <option key={k} value={k}>{label}</option>)}
            </Select>
          </Field>
        )}
        {v.allocationState !== keep && (
          <div className="sm:col-span-2">
            <Field label="Reason" htmlFor="bulk-ip-reason" hint="Needed when any record is released from allocated or reserved. Kept in each record's history.">
              <Input id="bulk-ip-reason" value={v.reason} onChange={set('reason')} />
            </Field>
          </div>
        )}
        <div className="sm:col-span-2"><FormError error={save.error} /></div>
      </form>
    </Modal>
  )
}

function SortHeader({ field, label, sort, onSort }: { field: string; label: string; sort: string; onSort: (s: string) => void }) {
  const active = sort === field || sort === `-${field}`
  const descending = sort === `-${field}`
  return (
    <Th>
      <button type="button" onClick={() => onSort(active && !descending ? `-${field}` : field)} aria-label={`Sort by ${label}`} aria-pressed={active}
        className="inline-flex cursor-pointer items-center gap-1 uppercase hover:text-ink">
        {label}{active && (descending ? <ArrowDown size={12} aria-hidden /> : <ArrowUp size={12} aria-hidden />)}
      </button>
    </Th>
  )
}

function FilterSelect({ label, value, onChange, children }: { label: string; value: string; onChange: (v: string) => void; children: ReactNode }) {
  return <Select aria-label={label} className="w-44!" value={value} onChange={(e) => onChange(e.target.value)}>{children}</Select>
}

export function IpExplorer() {
  const canCreate = useCan('inventory:create')
  const canEdit = useCan('inventory:edit')
  const filters = useUrlFilters()
  const { params, get, set } = filters
  const view = get('view') === 'grid' ? 'grid' : 'table'
  const open = get('open') || null
  const setOpen = (id: string | null) => set('open', id ?? '', false)
  const [creating, setCreating] = useState<{ address: string } | null>(null)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [bulk, setBulk] = useState(false)
  const showMore = moreKeys.some((k) => get(k)) || get('more') === '1'
  const domains = useRoutingDomains()
  const subnets = useOptions('/subnets')
  const sites = useOptions('/sites', showMore)
  const vlans = useOptions('/vlans', showMore)
  const vdoms = useOptions('/vdoms', showMore)
  const owners = useOptions('/users/lookup', showMore)
  const services = useOptions('/business-services', showMore)
  const cols = useColumns('ip-addresses', Object.keys(columns), ['device', 'lastSeen', 'service'])
  const v = cols.visible

  const paging = usePaging()
  const [search, setSearch] = useState(get('q'))
  const q = useDebounced(search)
  useEffect(() => setSearch(params.get('q') ?? ''), [params])
  useEffect(() => {
    if (q !== get('q')) set('q', q)
    // Only the debounced search value should trigger this.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q])

  const current = Object.fromEntries(filterKeys.map((k) => [k, get(k)]))
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, ...current })
  const records = useQuery({ queryKey: ['ip-addresses', qs], enabled: view === 'table', placeholderData: keepPreviousData, queryFn: () => list<IpAddress>(`/ip-addresses${qs}`) })
  useClampPage(paging, records.data?.meta)
  useEffect(() => setSelected(new Set()), [qs])
  const subnetId = get('subnetId')
  const subnet = useQuery({ queryKey: ['subnets', 'one', subnetId], enabled: Boolean(subnetId), queryFn: () => getOne<Subnet>(`/subnets/${subnetId}`) })
  const rows = records.data?.data ?? []
  const filtered = filterKeys.some((k) => k !== 'sort' && get(k))
  const allSelected = rows.length > 0 && rows.every((r) => selected.has(r.id))
  const toggle = (id: string) => setSelected((prev) => {
    const next = new Set(prev)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    return next
  })

  return (
    <>
      <PageHeader title="IP Address Explorer" description="Every tracked address, by routing domain. Allocation and observation are shown separately."
        actions={canCreate && <Button variant="primary" onClick={() => setCreating({ address: '' })}><Plus size={16} aria-hidden /> New record</Button>} />
      <Card>
        <div className="flex flex-wrap items-center gap-2 border-b border-line p-3">
          <div className="flex rounded-[5px] border border-line-strong p-0.5" role="group" aria-label="View">
            <button type="button" aria-pressed={view === 'table'} onClick={() => set('view', '', false)}
              className={`flex h-7 cursor-pointer items-center gap-1 rounded-[3px] px-2 text-[13px] ${view === 'table' ? 'bg-accent text-white' : 'text-ink-2 hover:text-ink'}`}><TableProperties size={14} aria-hidden /> Table</button>
            <button type="button" aria-pressed={view === 'grid'} onClick={() => set('view', 'grid', false)}
              className={`flex h-7 cursor-pointer items-center gap-1 rounded-[3px] px-2 text-[13px] ${view === 'grid' ? 'bg-accent text-white' : 'text-ink-2 hover:text-ink'}`}><Grid3x3 size={14} aria-hidden /> Grid</button>
          </div>
          <FilterSelect label="Subnet" value={subnetId} onChange={(x) => set('subnetId', x)}>
            <option value="">{view === 'grid' ? 'Choose a subnet' : 'All subnets'}</option>
            {subnets.data?.map((s) => <option key={s.id} value={s.id}>{(s as unknown as Subnet).calculated?.cidr ?? ''} {s.name}</option>)}
          </FilterSelect>
          {view === 'table' && (
            <>
              <div className="relative min-w-52 flex-1">
                <Search size={15} className="absolute top-1/2 left-2.5 -translate-y-1/2 text-ink-3" aria-hidden />
                <Input aria-label="Search by IP, hostname, FQDN or MAC" placeholder="Search by IP, hostname, FQDN or MAC" className="pl-8" value={search} onChange={(e) => setSearch(e.target.value)} />
              </div>
              <FilterSelect label="Routing domain" value={get('routingDomainId')} onChange={(x) => set('routingDomainId', x)}>
                <option value="">All routing domains</option>
                {domains.data?.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
              </FilterSelect>
              <FilterSelect label="Allocation state" value={get('allocationState')} onChange={(x) => set('allocationState', x)}>
                <option value="">Any allocation</option>
                {Object.entries(allocationStates).map(([k, label]) => <option key={k} value={k}>{label}</option>)}
              </FilterSelect>
              <FilterSelect label="Observation state" value={get('observationState')} onChange={(x) => set('observationState', x)}>
                <option value="">Any observation</option>
                {Object.entries(observationStates).map(([k, label]) => <option key={k} value={k}>{label}</option>)}
              </FilterSelect>
              <Button aria-expanded={showMore} onClick={() => set('more', showMore ? '' : '1', false)}><SlidersHorizontal size={15} aria-hidden /> More</Button>
              <SavedFilters page="ip-addresses" current={current} onApply={(f) => filters.replaceAll(f, ['pageSize'])} />
              <ColumnMenu columns={columns} visible={v} onToggle={cols.toggle} />
            </>
          )}
        </div>
        {view === 'table' && showMore && (
          <div className="flex flex-wrap items-center gap-2 border-b border-line bg-surface-2 p-3">
            <FilterSelect label="Address family" value={get('family')} onChange={(x) => set('family', x)}><option value="">IPv4 and IPv6</option><option value="4">IPv4</option><option value="6">IPv6</option></FilterSelect>
            <FilterSelect label="Site" value={get('siteId')} onChange={(x) => set('siteId', x)}><option value="">All sites</option>{sites.data?.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}</FilterSelect>
            <FilterSelect label="VLAN" value={get('vlanId')} onChange={(x) => set('vlanId', x)}><option value="">All VLANs</option>{vlans.data?.map((o) => <option key={o.id} value={o.id}>{optionLabel(o)}</option>)}</FilterSelect>
            <FilterSelect label="VDOM" value={get('vdomId')} onChange={(x) => set('vdomId', x)}><option value="">All VDOMs</option>{vdoms.data?.map((o) => <option key={o.id} value={o.id}>{optionLabel(o)}</option>)}</FilterSelect>
            <FilterSelect label="Owner" value={get('ownerId')} onChange={(x) => set('ownerId', x)}><option value="">Any owner</option>{owners.data?.map((o) => <option key={o.id} value={o.id}>{optionLabel(o)}</option>)}</FilterSelect>
            <FilterSelect label="Business service" value={get('businessServiceId')} onChange={(x) => set('businessServiceId', x)}><option value="">All services</option>{services.data?.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}</FilterSelect>
          </div>
        )}

        {view === 'grid' && (
          <div className="p-4">
            {!subnetId && <EmptyState title="Choose a subnet to see its address grid" body="The grid loads 256 addresses at a time, so even large IPv6 subnets stay fast." />}
            {subnetId && subnet.isPending && <SkeletonRows rows={4} />}
            {subnetId && subnet.error && <ErrorState error={subnet.error} onRetry={subnet.refetch} />}
            {subnet.data && <AddressGrid key={subnet.data.id} subnetId={subnet.data.id} info={subnet.data.calculated} onRecord={setOpen}
              onCandidate={canCreate && !subnet.data.archived ? (address) => setCreating({ address }) : undefined} />}
          </div>
        )}

        {view === 'table' && (
          <>
            {canEdit && (
              <SelectionBar count={selected.size} onClear={() => setSelected(new Set())}>
                <Button onClick={() => setBulk(true)}>Edit selected</Button>
              </SelectionBar>
            )}
            {records.isPending && <SkeletonRows rows={8} />}
            {records.error && <ErrorState error={records.error} onRetry={records.refetch} />}
            {records.data && rows.length === 0 && (filtered
              ? <EmptyState title="No records match these filters" />
              : <EmptyState title="No IP records yet" body="Records are created by hand here, from the subnet grid, or later by approved imports." />)}
            {records.data && rows.length > 0 && (
              <>
                <div className="overflow-x-auto">
                  <table className="w-full border-collapse">
                    <thead>
                      <tr>
                        {canEdit && <Th className="w-8"><input type="checkbox" aria-label="Select all on this page" className="size-4 accent-[var(--accent)]" checked={allSelected}
                          onChange={() => setSelected(allSelected ? new Set() : new Set(rows.map((r) => r.id)))} /></Th>}
                        <SortHeader field="address" label="Address" sort={get('sort')} onSort={(s) => set('sort', s)} />
                        {v('hostname') && <SortHeader field="hostname" label="Hostname" sort={get('sort')} onSort={(s) => set('sort', s)} />}
                        {v('state') && <Th>State</Th>}
                        {v('subnet') && <Th>Subnet</Th>}
                        {v('routingDomain') && <Th>Routing domain</Th>}
                        {v('mac') && <Th>MAC</Th>}
                        {v('owner') && <Th>Owner</Th>}
                        {v('service') && <Th>Business service</Th>}
                        {v('device') && <Th>Device</Th>}
                        {v('lastSeen') && <Th>Last seen</Th>}
                        {v('updated') && <SortHeader field="updatedAt" label="Updated" sort={get('sort')} onSort={(s) => set('sort', s)} />}
                      </tr>
                    </thead>
                    <tbody>
                      {rows.map((a) => (
                        <tr key={a.id} className={selected.has(a.id) ? 'bg-accent-soft/50' : 'hover:bg-surface-2'}>
                          {canEdit && <Td><input type="checkbox" aria-label={`Select ${a.address}`} className="size-4 accent-[var(--accent)]" checked={selected.has(a.id)} onChange={() => toggle(a.id)} /></Td>}
                          <Td><button type="button" className="cursor-pointer text-left text-ink hover:text-accent" onClick={() => setOpen(a.id)}><Mono>{a.address}</Mono></button></Td>
                          {v('hostname') && <Td className="text-ink-2">{a.hostname || '—'}</Td>}
                          {v('state') && <Td><StateBadges allocation={a.allocationState} observation={a.observationState} /></Td>}
                          {v('subnet') && <Td className="text-ink-2">{a.subnetName} <span className="text-ink-3"><Mono>{a.subnetCidr}</Mono></span></Td>}
                          {v('routingDomain') && <Td className="text-ink-2">{a.routingDomainName}</Td>}
                          {v('mac') && <Td className="text-ink-2">{a.mac ? <Mono>{a.mac}</Mono> : '—'}</Td>}
                          {v('owner') && <Td className="text-ink-2">{a.ownerEmail ?? '—'}</Td>}
                          {v('service') && <Td className="text-ink-2">{a.businessServiceName ?? '—'}</Td>}
                          {v('device') && <Td className="text-ink-2">{a.deviceName ?? '—'}</Td>}
                          {v('lastSeen') && <Td className="whitespace-nowrap text-ink-2">{a.lastSeen ? formatDateTime(a.lastSeen) : 'Never'}</Td>}
                          {v('updated') && <Td className="whitespace-nowrap text-ink-2">{formatDateTime(a.updatedAt)}</Td>}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <Pagination meta={records.data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
              </>
            )}
          </>
        )}
      </Card>
      <AddressDetail id={open} onClose={() => setOpen(null)} />
      <NewAddress open={creating !== null} onClose={() => setCreating(null)} address={creating?.address ?? ''}
        routingDomainId={subnet.data?.routingDomainId ?? get('routingDomainId')} onCreated={(a) => setOpen(a.id)} />
      {bulk && <BulkEdit ids={[...selected]} onClose={() => setBulk(false)} onDone={() => setSelected(new Set())} />}
    </>
  )
}
