import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { Search } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { RoutingDomainSelect } from '../../components/SearchSelect'
import { Badge, Card, EmptyState, ErrorState, Input, Meter, Mono, Pagination, Select, SkeletonRows } from '../../components/ui'
import { useDebounced } from '../../components/workflow'
import { list, queryString } from '../../lib/api'
import { environments } from '../../lib/format'
import { useClampPage, usePaging } from '../../lib/paging'
import type { Subnet } from '../../lib/types'

function SubnetCard({ subnet, onOpen }: { subnet: Subnet; onOpen: () => void }) {
  const place = [subnet.routingDomainName, subnet.site?.name].filter(Boolean).join(' · ')
  return (
    <button type="button" onClick={onOpen} className="flex min-w-0 cursor-pointer flex-col gap-1.5 rounded-md border border-line bg-surface p-3 text-left transition-colors hover:border-accent focus-visible:border-accent focus-visible:outline-none">
      <span className="flex items-start justify-between gap-2">
        <span className="min-w-0 font-semibold break-all text-ink"><Mono>{subnet.calculated.cidr}</Mono></span>
        <Badge>{environments[subnet.environment] ?? subnet.environment}</Badge>
      </span>
      <span className="truncate font-medium text-ink">{subnet.name}</span>
      <span className="truncate text-[12.5px] text-ink-2">{place}</span>
      <span className="mt-auto pt-1"><Meter pct={Number(subnet.utilisationPct)} level={subnet.utilisationLevel} label={`${subnet.name} utilisation`} /></span>
      <span className="text-[12px] text-ink-3 tabular-nums">{subnet.allocated} allocated · {subnet.reserved} reserved</span>
    </button>
  )
}

/** The way into the explorer: find a subnet by name, CIDR or an address inside it. */
export function SubnetCards({ onOpen }: { onOpen: (s: Subnet) => void }) {
  const [params, setParams] = useSearchParams()
  const paging = usePaging('subnets', 24)
  const [text, setText] = useState(params.get('subnetQ') ?? '')
  const q = useDebounced(text.trim())
  const routingDomainId = params.get('routingDomainId') ?? ''
  const environment = params.get('environment') ?? ''
  const set = (key: string, value: string) => setParams((prev) => {
    const next = new URLSearchParams(prev)
    if (value) next.set(key, value)
    else next.delete(key)
    next.delete('subnetsPage')
    return next
  }, { replace: true })
  useEffect(() => {
    if (q !== (params.get('subnetQ') ?? '')) set('subnetQ', q)
    // Only a settled search should touch the address.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q])
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, q, routingDomainId, environment, sort: 'cidr' })
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['subnets', 'cards', qs], placeholderData: keepPreviousData, queryFn: () => list<Subnet>(`/subnets${qs}`) })
  useClampPage(paging, data?.meta)
  const filtered = Boolean(q || routingDomainId || environment)
  return (
    <Card>
      <div className="flex flex-wrap items-center gap-2 border-b border-line p-2.5">
        <div className="relative min-w-64 flex-1">
          <Search size={14} className="absolute top-1/2 left-2.5 -translate-y-1/2 text-ink-3" aria-hidden />
          <Input aria-label="Search subnets" placeholder="Name, CIDR such as 10.20.0.0/16, or an IP address" className="pl-8" value={text} onChange={(e) => setText(e.target.value)} />
        </div>
        <div className="w-56"><RoutingDomainSelect aria-label="Routing domain" emptyLabel="Any routing domain" value={routingDomainId} onChange={(v) => set('routingDomainId', v)} /></div>
        <Select aria-label="Environment" className="w-44!" value={environment} onChange={(e) => set('environment', e.target.value)}>
          <option value="">Any environment</option>
          {Object.entries(environments).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
        </Select>
      </div>
      {isPending && <SkeletonRows rows={6} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && (filtered
        ? <EmptyState title="No subnets match" body="A CIDR finds the subnets inside it and around it. An IP address finds the subnet holding it." />
        : <EmptyState title="No subnets yet" body="Add one first, then its addresses appear here." action={<Link className="text-accent hover:underline" to="/subnets">Go to Subnet Management</Link>} />)}
      {data && data.data.length > 0 && (
        <>
          <div className="grid gap-2.5 p-2.5 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {data.data.map((s) => <SubnetCard key={s.id} subnet={s} onOpen={() => onOpen(s)} />)}
          </div>
          <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
        </>
      )}
    </Card>
  )
}
