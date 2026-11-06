import { SubnetTable } from './SubnetTable'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Columns3, Layers, Plus, Search, Wrench } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router'
import { SavedFilters } from '../../components/SavedFilters'
import { RoutingDomainSelect } from '../../components/SearchSelect'
import { Button, Card, ConfirmDialog, EmptyState, ErrorState, Input, PageHeader, Pagination, Select, SkeletonRows, toast } from '../../components/ui'
import { useDebounced } from '../../components/workflow'
import { columnLabels, useColumns } from './columns'
import type { Column } from './columns'
import { CreateModal } from './CreateModal'
import { DetailModal } from './DetailModal'
import { get, list, post, queryString, errorMessage } from '../../lib/api'
import { environments } from '../../lib/format'
import { useClampPage, usePaging } from '../../lib/paging'
import { useCan } from '../../lib/session'
import type { Subnet } from '../../lib/types'
import { ToolsModal } from './ToolsModal'
import { BulkCreateModal } from './BulkCreateModal'

function ColumnPicker({ columns, onToggle }: { columns: Set<Column>; onToggle: (c: Column) => void }) {
  const [open, setOpen] = useState(false)
  return (
    <div className="relative ml-auto">
      <Button variant="ghost" aria-expanded={open} onClick={() => setOpen((v) => !v)}><Columns3 size={14} aria-hidden /> Columns</Button>
      {open && (
        <div className="absolute right-0 z-20 mt-1 w-48 rounded-md border border-line-strong/70 bg-surface p-2 shadow-[0_12px_32px_-12px_rgba(11,18,32,.3)]">
          {(Object.keys(columnLabels) as Column[]).map((c) => (
            <label key={c} className="flex items-center gap-2 py-0.5 text-[13px] text-ink"><input type="checkbox" checked={columns.has(c)} onChange={() => onToggle(c)} /> {columnLabels[c]}</label>
          ))}
        </div>
      )}
    </div>
  )
}

export function Subnets() {
  const canCreate = useCan('inventory:create')
  const canEdit = useCan('inventory:edit')
  const queryClient = useQueryClient()
  const [columns, toggleColumn] = useColumns()
  const [tool, setTool] = useState<'bulk' | 'tools' | null>(null)
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const [archiving, setArchiving] = useState(false)
  const bulkArchive = useMutation({ meta: { success: 'Subnets archived.', inlineError: true },
    mutationFn: () => post('/subnets/bulk-archive', { ids: [...picked] }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['subnets'] })
      queryClient.invalidateQueries({ queryKey: ['summary'] })
      setPicked(new Set())
      setArchiving(false)
    },
  })
  const [params, setParams] = useSearchParams()
  const [creating, setCreating] = useState(false)
  const [selected, setSelected] = useState<Subnet | null>(null)

  const paging = usePaging()
  const [search, setSearch] = useState(params.get('q') ?? '')
  const q = useDebounced(search)
  const family = params.get('family') ?? ''
  const environment = params.get('environment') ?? ''
  const routingDomainId = params.get('routingDomainId') ?? ''

  useEffect(() => setSearch(params.get('q') ?? ''), [params])

  const setFilter = (key: string, value: string) => {
    setParams((prev) => {
      const next = new URLSearchParams(prev)
      if (value) next.set(key, value)
      else next.delete(key)
      if (key !== 'page') next.delete('page')
      return next
    })
  }

  useEffect(() => {
    if (q !== (params.get('q') ?? '')) setFilter('q', q)
    // Only the debounced search value should trigger this.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q])

  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, q: params.get('q') ?? '', family, environment, routingDomainId })
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['subnets', qs],
    queryFn: () => list<Subnet>(`/subnets${qs}`),
    placeholderData: keepPreviousData,
  })
  useClampPage(paging, data?.meta)
  const filtered = Boolean(params.get('q') || family || environment || routingDomainId)

  return (
    <>
      <PageHeader
        title="Subnet Management"
        description="IPv4 and IPv6 subnets by routing domain."
        actions={<>
          <Button onClick={() => setTool('tools')}><Wrench size={14} aria-hidden /> Tools</Button>
          {canCreate && <Button onClick={() => setTool('bulk')}><Layers size={14} aria-hidden /> Bulk create</Button>}
          {canCreate && <Button variant="primary" onClick={() => setCreating(true)}><Plus size={16} aria-hidden /> New subnet</Button>}
        </>}
      />
      <Card>
        <div className="flex flex-wrap items-center gap-2 border-b border-line p-3">
          <div className="relative min-w-52 flex-1">
            <Search size={15} className="absolute top-1/2 left-2.5 -translate-y-1/2 text-ink-3" aria-hidden />
            <Input aria-label="Filter by name or CIDR" placeholder="Filter by name or CIDR" className="pl-8" value={search} onChange={(e) => setSearch(e.target.value)} />
          </div>
          <Select aria-label="Address family" className="w-40!" value={family} onChange={(e) => setFilter('family', e.target.value)}>
            <option value="">IPv4 and IPv6</option><option value="4">IPv4</option><option value="6">IPv6</option>
          </Select>
          <Select aria-label="Environment" className="w-44!" value={environment} onChange={(e) => setFilter('environment', e.target.value)}>
            <option value="">All environments</option>
            {Object.entries(environments).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </Select>
          <RoutingDomainSelect aria-label="Routing domain" className="w-56" emptyLabel="All routing domains" value={routingDomainId} onChange={(v) => setFilter('routingDomainId', v)} />
          <SavedFilters page="subnets" current={{ q: params.get('q') ?? '', family, environment, routingDomainId }}
            onApply={(f) => setParams(new URLSearchParams(f))} />
          <ColumnPicker columns={columns} onToggle={toggleColumn} />
        </div>
        {canEdit && picked.size > 0 && (
          <div className="flex items-center gap-2 border-b border-line bg-accent-soft/40 px-3 py-1.5 text-[13px]">
            <span>{picked.size} selected</span>
            <Button className="h-7" onClick={() => { bulkArchive.reset(); setArchiving(true) }}>Archive selected</Button>
            <Button variant="ghost" className="h-7" onClick={() => setPicked(new Set())}>Clear</Button>
          </div>
        )}
        {isPending && <SkeletonRows rows={8} />}
        {error && <ErrorState error={error} onRetry={refetch} />}
        {data && data.data.length === 0 && (
          filtered
            ? <EmptyState title="No subnets match these filters" />
            : <EmptyState title="No subnets yet" body="Create a subnet to start tracking capacity and utilisation." />
        )}
        {data && data.data.length > 0 && (
          <>
            <SubnetTable rows={data.data} columns={columns} selectable={canEdit} picked={picked} onPick={setPicked} onOpen={setSelected} />
            <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
          </>
        )}
      </Card>
      <CreateModal open={creating} onClose={() => setCreating(false)} />
      <DetailModal subnet={selected} onClose={() => setSelected(null)} />
      {tool === 'bulk' && <BulkCreateModal onClose={() => setTool(null)} />}
      {tool === 'tools' && <ToolsModal onClose={() => setTool(null)} onOpenSubnet={(id) => { setTool(null); get<Subnet>(`/subnets/${id}`).then(setSelected, (e) => toast.error(errorMessage(e))) }} />}
      <ConfirmDialog open={archiving} title={`Archive ${picked.size} subnets?`}
        body="They leave the active inventory together, and their ranges can be used again. History is kept."
        confirmLabel="Archive" busy={bulkArchive.isPending} error={bulkArchive.error} onConfirm={() => bulkArchive.mutate()} onCancel={() => setArchiving(false)} />
    </>
  )
}
