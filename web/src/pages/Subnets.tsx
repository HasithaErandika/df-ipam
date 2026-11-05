import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { ArrowDown, ArrowUp, Calculator, Plus, Search, SlidersHorizontal } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { ColumnMenu, SavedFilters, SelectionBar } from '../components/inventory'
import { Badge, Button, Card, EmptyState, ErrorState, Input, Meter, Mono, PageHeader, Pagination, Select, SkeletonRows, Td, Th } from '../components/ui'
import { list, queryString } from '../lib/api'
import { environments, formatCapacity, formatDateTime } from '../lib/format'
import { optionLabel, useColumns, useDebounced, useOptions, useUrlFilters } from '../lib/hooks'
import { useClampPage, usePaging } from '../lib/paging'
import { useCan } from '../lib/session'
import type { Subnet } from '../lib/types'
import { BulkConfirm, BulkEdit } from './subnets/Bulk'
import { SubnetDetail } from './subnets/SubnetDetail'
import { SubnetForm, useRoutingDomains } from './subnets/SubnetForm'
import { Tools } from './subnets/Tools'

const filterKeys = ['q', 'family', 'environment', 'routingDomainId', 'siteId', 'vlanId', 'vdomId', 'businessServiceId', 'networkBlockId', 'includeArchived', 'sort'] as const
const moreKeys = ['siteId', 'vlanId', 'vdomId', 'businessServiceId', 'networkBlockId', 'includeArchived']

const columns = {
  routingDomain: 'Routing domain', environment: 'Environment', site: 'Site', vlan: 'VLAN', gateway: 'Gateway',
  usable: 'Usable', utilisation: 'Utilisation', updated: 'Updated',
}

function SortHeader({ field, label, sort, onSort, className }: { field: string; label: string; sort: string; onSort: (s: string) => void; className?: string }) {
  const active = sort === field || sort === `-${field}`
  const descending = sort === `-${field}`
  return (
    <Th className={className}>
      <button type="button" onClick={() => onSort(active && !descending ? `-${field}` : field)} className="inline-flex cursor-pointer items-center gap-1 uppercase hover:text-ink"
        aria-label={`Sort by ${label}`} aria-pressed={active}>
        {label}{active && (descending ? <ArrowDown size={12} aria-hidden /> : <ArrowUp size={12} aria-hidden />)}
      </button>
    </Th>
  )
}

function FilterSelect({ label, value, onChange, children }: { label: string; value: string; onChange: (v: string) => void; children: ReactNode }) {
  return <Select aria-label={label} className="w-44!" value={value} onChange={(e) => onChange(e.target.value)}>{children}</Select>
}

export function Subnets() {
  const canCreate = useCan('inventory:create')
  const canEdit = useCan('inventory:edit')
  const canDelete = useCan('inventory:delete')
  const filters = useUrlFilters()
  const { params, get, set } = filters
  const [creating, setCreating] = useState(false)
  const [tools, setTools] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [bulk, setBulk] = useState<'edit' | 'archive' | 'delete' | null>(null)
  const open = get('open') || null
  const setOpen = (id: string | null) => set('open', id ?? '', false)
  const domains = useRoutingDomains()
  const showMore = moreKeys.some((k) => get(k)) || params.get('more') === '1'
  const sites = useOptions('/sites', showMore)
  const vlans = useOptions('/vlans', showMore)
  const vdoms = useOptions('/vdoms', showMore)
  const services = useOptions('/business-services', showMore)
  const blocks = useOptions('/network-blocks', showMore)
  const cols = useColumns('subnets', Object.keys(columns), ['site', 'vlan', 'updated'])

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
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['subnets', qs], queryFn: () => list<Subnet>(`/subnets${qs}`), placeholderData: keepPreviousData })
  useClampPage(paging, data?.meta)
  useEffect(() => setSelected(new Set()), [qs])
  const filtered = filterKeys.some((k) => k !== 'sort' && get(k))
  const rows = data?.data ?? []
  const allSelected = rows.length > 0 && rows.every((s) => selected.has(s.id))
  const toggle = (id: string) => setSelected((prev) => {
    const next = new Set(prev)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    return next
  })
  const canBulk = canEdit || canDelete
  const v = cols.visible

  return (
    <>
      <PageHeader
        title="Subnet Management"
        description="IPv4 and IPv6 subnets by routing domain."
        actions={<>
          <Button onClick={() => setTools(true)}><Calculator size={15} aria-hidden /> Tools</Button>
          {canCreate && <Button variant="primary" onClick={() => setCreating(true)}><Plus size={16} aria-hidden /> New subnet</Button>}
        </>}
      />
      <Card>
        <div className="flex flex-wrap items-center gap-2 border-b border-line p-3">
          <div className="relative min-w-52 flex-1">
            <Search size={15} className="absolute top-1/2 left-2.5 -translate-y-1/2 text-ink-3" aria-hidden />
            <Input aria-label="Filter by name or CIDR" placeholder="Filter by name or CIDR" className="pl-8" value={search} onChange={(e) => setSearch(e.target.value)} />
          </div>
          <FilterSelect label="Address family" value={get('family')} onChange={(x) => set('family', x)}>
            <option value="">IPv4 and IPv6</option><option value="4">IPv4</option><option value="6">IPv6</option>
          </FilterSelect>
          <FilterSelect label="Environment" value={get('environment')} onChange={(x) => set('environment', x)}>
            <option value="">All environments</option>
            {Object.entries(environments).map(([k, label]) => <option key={k} value={k}>{label}</option>)}
          </FilterSelect>
          <FilterSelect label="Routing domain" value={get('routingDomainId')} onChange={(x) => set('routingDomainId', x)}>
            <option value="">All routing domains</option>
            {domains.data?.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
          </FilterSelect>
          <Button aria-expanded={showMore} onClick={() => set('more', showMore ? '' : '1', false)}><SlidersHorizontal size={15} aria-hidden /> More</Button>
          <SavedFilters page="subnets" current={current} onApply={(f) => filters.replaceAll(f, ['pageSize'])} />
          <ColumnMenu columns={columns} visible={v} onToggle={cols.toggle} />
        </div>
        {showMore && (
          <div className="flex flex-wrap items-center gap-2 border-b border-line bg-surface-2 p-3">
            <FilterSelect label="Site" value={get('siteId')} onChange={(x) => set('siteId', x)}><option value="">All sites</option>{sites.data?.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}</FilterSelect>
            <FilterSelect label="VLAN" value={get('vlanId')} onChange={(x) => set('vlanId', x)}><option value="">All VLANs</option>{vlans.data?.map((o) => <option key={o.id} value={o.id}>{optionLabel(o)}</option>)}</FilterSelect>
            <FilterSelect label="VDOM" value={get('vdomId')} onChange={(x) => set('vdomId', x)}><option value="">All VDOMs</option>{vdoms.data?.map((o) => <option key={o.id} value={o.id}>{optionLabel(o)}</option>)}</FilterSelect>
            <FilterSelect label="Business service" value={get('businessServiceId')} onChange={(x) => set('businessServiceId', x)}><option value="">All services</option>{services.data?.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}</FilterSelect>
            <FilterSelect label="Network block" value={get('networkBlockId')} onChange={(x) => set('networkBlockId', x)}><option value="">All blocks</option>{blocks.data?.map((o) => <option key={o.id} value={o.id}>{optionLabel(o)}</option>)}</FilterSelect>
            <label className="flex items-center gap-2 text-[13px] text-ink">
              <input type="checkbox" className="size-4 accent-[var(--accent)]" checked={get('includeArchived') === 'true'} onChange={(e) => set('includeArchived', e.target.checked ? 'true' : '')} />
              Include archived
            </label>
          </div>
        )}
        {canBulk && (
          <SelectionBar count={selected.size} onClear={() => setSelected(new Set())}>
            {canEdit && <Button onClick={() => setBulk('edit')}>Edit fields</Button>}
            {canEdit && <Button onClick={() => setBulk('archive')}>Archive</Button>}
            {canDelete && <Button variant="danger" onClick={() => setBulk('delete')}>Delete</Button>}
          </SelectionBar>
        )}
        {isPending && <SkeletonRows rows={8} />}
        {error && <ErrorState error={error} onRetry={refetch} />}
        {data && rows.length === 0 && (
          filtered
            ? <EmptyState title="No subnets match these filters" />
            : <EmptyState title="No subnets yet" body="Create a subnet to start tracking capacity and utilisation." />
        )}
        {data && rows.length > 0 && (
          <>
            <div className="overflow-x-auto">
              <table className="w-full border-collapse">
                <thead>
                  <tr>
                    {canBulk && <Th className="w-8"><input type="checkbox" aria-label="Select all on this page" className="size-4 accent-[var(--accent)]" checked={allSelected}
                      onChange={() => setSelected(allSelected ? new Set() : new Set(rows.filter((s) => !s.archived).map((s) => s.id)))} /></Th>}
                    <SortHeader field="name" label="Name" sort={get('sort')} onSort={(s) => set('sort', s)} />
                    <SortHeader field="cidr" label="CIDR" sort={get('sort')} onSort={(s) => set('sort', s)} />
                    {v('routingDomain') && <Th>Routing domain</Th>}
                    {v('environment') && <SortHeader field="environment" label="Environment" sort={get('sort')} onSort={(s) => set('sort', s)} />}
                    {v('site') && <Th>Site</Th>}
                    {v('vlan') && <Th>VLAN</Th>}
                    {v('gateway') && <Th>Gateway</Th>}
                    {v('usable') && <Th className="text-right">Usable</Th>}
                    {v('utilisation') && <SortHeader field="utilisation" label="Utilisation" sort={get('sort')} onSort={(s) => set('sort', s)} />}
                    {v('updated') && <SortHeader field="updatedAt" label="Updated" sort={get('sort')} onSort={(s) => set('sort', s)} />}
                  </tr>
                </thead>
                <tbody>
                  {rows.map((s) => (
                    <tr key={s.id} className={selected.has(s.id) ? 'bg-accent-soft/50' : 'hover:bg-surface-2'}>
                      {canBulk && <Td><input type="checkbox" aria-label={`Select ${s.name}`} className="size-4 accent-[var(--accent)]" disabled={s.archived} checked={selected.has(s.id)} onChange={() => toggle(s.id)} /></Td>}
                      <Td>
                        <button type="button" className="cursor-pointer text-left font-medium text-ink hover:text-accent" onClick={() => setOpen(s.id)}>{s.name}</button>
                        {s.archived && <> <Badge>Archived</Badge></>}
                      </Td>
                      <Td><Mono>{s.calculated.cidr}</Mono></Td>
                      {v('routingDomain') && <Td className="text-ink-2">{s.routingDomainName}</Td>}
                      {v('environment') && <Td><Badge tone={s.environment === 'production' ? 'accent' : 'neutral'}>{environments[s.environment]}</Badge></Td>}
                      {v('site') && <Td className="text-ink-2">{s.site?.name ?? '—'}</Td>}
                      {v('vlan') && <Td className="text-ink-2">{s.vlan?.name ?? '—'}</Td>}
                      {v('gateway') && <Td className="text-ink-2">{s.gateway ? <Mono>{s.gateway}</Mono> : '—'}</Td>}
                      {v('usable') && <Td className="text-right text-ink-2 tabular-nums">{formatCapacity(s.calculated.usableAddresses, s.calculated.family, s.calculated.prefixLength)}</Td>}
                      {v('utilisation') && <Td><Meter pct={Number(s.utilisationPct)} level={s.utilisationLevel} label={`${s.name} utilisation`} /></Td>}
                      {v('updated') && <Td className="whitespace-nowrap text-ink-2">{formatDateTime(s.updatedAt)}</Td>}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
          </>
        )}
      </Card>
      {creating && <SubnetForm subnet={null} onClose={() => setCreating(false)} onSaved={(s) => setOpen(s.id)} />}
      {tools && <Tools onClose={() => setTools(false)} onOpen={setOpen} />}
      <SubnetDetail id={open} onClose={() => setOpen(null)} />
      {bulk === 'edit' && <BulkEdit ids={[...selected]} onClose={() => setBulk(null)} onDone={() => setSelected(new Set())} />}
      {(bulk === 'archive' || bulk === 'delete') && <BulkConfirm ids={[...selected]} action={bulk} onClose={() => setBulk(null)} onDone={() => setSelected(new Set())} />}
    </>
  )
}
