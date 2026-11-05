import { zodResolver } from '@hookform/resolvers/zod'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, Search } from 'lucide-react'
import { useEffect, useState, type ReactNode } from 'react'
import { useForm } from 'react-hook-form'
import { useSearchParams } from 'react-router'
import { z } from 'zod'
import {
  Badge, Button, Card, ConfirmDialog, Modal, EmptyState, ErrorState, Field, FormError, Input, Meter, Mono, PageHeader,
  Pagination, Select, SkeletonRows, Td, Th,
} from '../components/ui'
import { ApiError, get, list, listAll, post, queryString } from '../lib/api'
import { environments, formatCapacity, formatDateTime } from '../lib/format'
import { useClampPage, usePaging } from '../lib/paging'
import { useCan } from '../lib/session'
import type { FreeRanges, NetInfo, RoutingDomain, Subnet, Utilisation } from '../lib/types'

function useDebounced<T>(value: T, ms = 300): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return debounced
}

interface Option { id: string; name: string; code?: string; vlanId?: number; cidr?: string }

const useOptions = (path: string, enabled = true) =>
  useQuery({ queryKey: ['options', path], enabled, queryFn: () => listAll<Option>(path) })

const useRoutingDomains = () =>
  useQuery({ queryKey: ['routing-domains', 'all'], queryFn: () => listAll<RoutingDomain>('/routing-domains') })

function Detail({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex justify-between gap-4 border-b border-line py-1.5 last:border-0">
      <dt className="text-ink-2">{label}</dt>
      <dd className="text-right text-ink">{children}</dd>
    </div>
  )
}

function Calculated({ info }: { info: NetInfo }) {
  return (
    <dl>
      <Detail label="Network"><Mono>{info.network}</Mono></Detail>
      {info.mask && <Detail label="Subnet mask"><Mono>{info.mask}</Mono></Detail>}
      {info.broadcast && <Detail label="Broadcast"><Mono>{info.broadcast}</Mono></Detail>}
      <Detail label="First usable"><Mono>{info.firstUsable}</Mono></Detail>
      <Detail label="Last usable"><Mono>{info.lastUsable}</Mono></Detail>
      <Detail label="Total addresses"><span className="tabular-nums">{formatCapacity(info.totalAddresses, info.family, info.prefixLength)}</span></Detail>
      <Detail label="Usable addresses"><span className="tabular-nums">{formatCapacity(info.usableAddresses, info.family, info.prefixLength)}</span></Detail>
    </dl>
  )
}

const schema = z.object({
  name: z.string().trim().min(1, 'Enter a subnet name.'),
  cidr: z.string().trim().min(1, 'Enter a CIDR.'),
  routingDomainId: z.string().min(1, 'Choose a routing domain.'),
  environment: z.string().min(1, 'Choose an environment.'),
  gateway: z.string().trim(),
  securityZone: z.string().trim(),
  description: z.string().trim(),
  siteId: z.string(),
  vlanId: z.string(),
  businessServiceId: z.string(),
  networkBlockId: z.string(),
})
type FormValues = z.infer<typeof schema>

interface Preview { info: NetInfo; conflicts: { id: string; name: string; cidr: string }[] }

function CreateModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const queryClient = useQueryClient()
  const domains = useRoutingDomains()
  const sites = useOptions('/sites', open)
  const vlans = useOptions('/vlans', open)
  const services = useOptions('/business-services', open)
  const blocks = useOptions('/network-blocks', open)
  const { register, handleSubmit, watch, reset, setError, formState: { errors } } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '', cidr: '', routingDomainId: '', environment: '', gateway: '', securityZone: '', description: '', siteId: '', vlanId: '', businessServiceId: '', networkBlockId: '' },
  })
  const cidr = useDebounced(watch('cidr').trim())
  const routingDomainId = watch('routingDomainId')

  const preview = useQuery({
    queryKey: ['subnet-preview', cidr, routingDomainId],
    enabled: open && cidr.includes('/'),
    retry: false,
    queryFn: async (): Promise<Preview> => {
      const info = await post<NetInfo>('/subnets/calculate', { cidr })
      if (!routingDomainId) return { info, conflicts: [] }
      const { conflicts } = await post<{ conflicts: Preview['conflicts'] }>('/subnets/check-overlap', { cidr, routingDomainId })
      return { info, conflicts }
    },
  })

  const create = useMutation({
    mutationFn: (v: FormValues) => post<Subnet>('/subnets', {
      ...v,
      gateway: v.gateway || null,
      siteId: v.siteId || null,
      vlanId: v.vlanId || null,
      businessServiceId: v.businessServiceId || null,
      networkBlockId: v.networkBlockId || null,
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['subnets'] })
      queryClient.invalidateQueries({ queryKey: ['summary'] })
      reset()
      create.reset()
      onClose()
    },
    onError: (e) => {
      if (e instanceof ApiError) {
        for (const d of e.details) setError(d.field as keyof FormValues, { message: d.issue })
      }
    },
  })

  const previewIssue = preview.error instanceof ApiError ? preview.error.details[0]?.issue : undefined
  const fieldLevel = create.error instanceof ApiError && create.error.details.length > 0

  return (
    <Modal
      open={open}
      onClose={onClose}
      size="xl"
      title="New subnet"
      footer={<>
        <Button onClick={onClose}>Cancel</Button>
        <Button variant="primary" type="submit" form="new-subnet" busy={create.isPending}>Create subnet</Button>
      </>}
    >
      <form id="new-subnet" noValidate onSubmit={handleSubmit((v) => create.mutate(v))} className="grid gap-5 md:grid-cols-[1fr_17rem]">
        <div className="space-y-3.5">
          <div className="grid grid-cols-2 gap-3">
            <Field label="Name" htmlFor="name" error={errors.name?.message}><Input id="name" autoFocus {...register('name')} /></Field>
            <Field label="Environment" htmlFor="environment" error={errors.environment?.message}>
              <Select id="environment" {...register('environment')}>
                <option value="">Select</option>
                {Object.entries(environments).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
              </Select>
            </Field>
            <Field label="CIDR" htmlFor="cidr" error={errors.cidr?.message ?? previewIssue}>
              <Input id="cidr" className="font-mono" spellCheck={false} placeholder="10.20.30.0/24" {...register('cidr')} />
            </Field>
            <Field label="Gateway (optional)" htmlFor="gateway" error={errors.gateway?.message}>
              <Input id="gateway" className="font-mono" spellCheck={false} {...register('gateway')} />
            </Field>
          </div>
          <Field label="Routing domain" htmlFor="routingDomainId" error={errors.routingDomainId?.message} hint="Overlaps are only blocked within the same routing domain.">
            <Select id="routingDomainId" {...register('routingDomainId')}>
              <option value="">Select</option>
              {domains.data?.map((d) => <option key={d.id} value={d.id}>{d.organisationName} / {d.name}</option>)}
            </Select>
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Site (optional)" htmlFor="siteId" error={errors.siteId?.message}>
              <Select id="siteId" {...register('siteId')}><option value="">None</option>{sites.data?.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}</Select>
            </Field>
            <Field label="VLAN (optional)" htmlFor="vlanId" error={errors.vlanId?.message}>
              <Select id="vlanId" {...register('vlanId')}><option value="">None</option>{vlans.data?.map((o) => <option key={o.id} value={o.id}>{o.vlanId} {o.name}</option>)}</Select>
            </Field>
            <Field label="Business service (optional)" htmlFor="businessServiceId">
              <Select id="businessServiceId" {...register('businessServiceId')}><option value="">None</option>{services.data?.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}</Select>
            </Field>
            <Field label="Network block (optional)" htmlFor="networkBlockId" error={errors.networkBlockId?.message}>
              <Select id="networkBlockId" {...register('networkBlockId')}><option value="">None</option>{blocks.data?.map((o) => <option key={o.id} value={o.id}>{o.cidr} {o.name}</option>)}</Select>
            </Field>
            <Field label="Security zone (optional)" htmlFor="securityZone"><Input id="securityZone" {...register('securityZone')} /></Field>
            <Field label="Description (optional)" htmlFor="description"><Input id="description" {...register('description')} /></Field>
          </div>
          {!fieldLevel && <FormError error={create.error} />}
        </div>
        <aside className="self-start rounded-[5px] border border-line bg-surface-2 px-3 py-2">
          <h3 className="mb-1 text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase">Calculated</h3>
          {preview.data ? <Calculated info={preview.data.info} /> : <p className="py-2 text-[13px] text-ink-3">Enter a CIDR to see its range and capacity.</p>}
          {preview.data && preview.data.conflicts.length > 0 && (
            <p role="alert" className="mt-2 rounded-[4px] bg-danger-soft px-2 py-1.5 text-[12.5px] text-danger">
              Overlaps {preview.data.conflicts.map((c) => `${c.cidr} (${c.name})`).join(', ')}.
            </p>
          )}
        </aside>
      </form>
    </Modal>
  )
}

const heading = 'mb-1 text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase'

function DetailModal({ subnet, onClose }: { subnet: Subnet | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const canEdit = useCan('inventory:edit')
  const [confirming, setConfirming] = useState(false)
  const archive = useMutation({
    mutationFn: (id: string) => post<void>(`/subnets/${id}/archive`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['subnets'] })
      queryClient.invalidateQueries({ queryKey: ['summary'] })
      setConfirming(false)
      onClose()
    },
  })
  const usage = useQuery({ queryKey: ['subnets', subnet?.id, 'utilisation'], enabled: subnet !== null, queryFn: () => get<Utilisation>(`/subnets/${subnet?.id}/utilisation`) })
  const free = useQuery({ queryKey: ['subnets', subnet?.id, 'free'], enabled: subnet !== null, queryFn: () => get<FreeRanges>(`/subnets/${subnet?.id}/free-ranges`) })
  if (!subnet) return null
  return (
    <>
      <Modal
        open
        size="xl"
        onClose={onClose}
        title={subnet.name}
        subtitle={<Mono>{subnet.calculated.cidr}</Mono>}
        footer={canEdit && !subnet.archived ? <Button variant="secondary" onClick={() => setConfirming(true)}>Archive subnet</Button> : undefined}
      >
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
                <Detail label="Available candidates"><span className="tabular-nums">{formatCapacity(usage.data.availableCandidates, subnet.calculated.family, subnet.calculated.prefixLength)}</span></Detail>
              </dl>
            )}
            <h3 className={`${heading} mt-4`}>Addressing</h3>
            <Calculated info={subnet.calculated} />
          </section>
          <section>
            <h3 className={heading}>Mapping</h3>
            <dl>
              <Detail label="Routing domain">{subnet.routingDomainName}</Detail>
              <Detail label="Environment">{environments[subnet.environment]}</Detail>
              <Detail label="Network block">{subnet.networkBlock?.name ?? 'Not set'}</Detail>
              <Detail label="Site">{subnet.site?.name ?? 'Not set'}</Detail>
              <Detail label="VLAN">{subnet.vlan?.name ?? 'Not set'}</Detail>
              <Detail label="VDOM">{subnet.vdom?.name ?? 'Not set'}</Detail>
              <Detail label="Business service">{subnet.businessService?.name ?? 'Not set'}</Detail>
              <Detail label="Technical owner">{subnet.technicalOwner?.name ?? 'Not set'}</Detail>
              <Detail label="Business owner">{subnet.businessOwner?.name ?? 'Not set'}</Detail>
              <Detail label="Gateway">{subnet.gateway ? <Mono>{subnet.gateway}</Mono> : 'Not set'}</Detail>
              <Detail label="Security zone">{subnet.securityZone || 'Not set'}</Detail>
              <Detail label="Updated">{formatDateTime(subnet.updatedAt)}</Detail>
            </dl>
            <h3 className={`${heading} mt-4`}>Free ranges</h3>
            {free.data?.ranges.length === 0 && <p className="text-[13px] text-ink-3">No free ranges.</p>}
            {free.data && free.data.ranges.length > 0 && (
              <ul className="max-h-40 overflow-y-auto">
                {free.data.ranges.map((r) => (
                  <li key={r.first} className="flex justify-between gap-3 border-b border-line py-1 last:border-0">
                    <Mono>{r.first === r.last ? r.first : `${r.first} – ${r.last}`}</Mono>
                  </li>
                ))}
              </ul>
            )}
            {free.data?.truncated && <p className="mt-1 text-[12px] text-ink-3">Showing the first {free.data.ranges.length} of {free.data.rangeCount}.</p>}
          </section>
        </div>
        {subnet.description && <p className="mt-4 text-ink-2">{subnet.description}</p>}
      </Modal>
      <ConfirmDialog
        open={confirming}
        title="Archive this subnet?"
        body={`${subnet.calculated.cidr} leaves the active inventory, and its range can be used again. History is kept.`}
        confirmLabel="Archive"
        busy={archive.isPending}
        error={archive.error}
        onConfirm={() => archive.mutate(subnet.id)}
        onCancel={() => setConfirming(false)}
      />
    </>
  )
}

export function Subnets() {
  const canCreate = useCan('inventory:create')
  const [params, setParams] = useSearchParams()
  const [creating, setCreating] = useState(false)
  const [selected, setSelected] = useState<Subnet | null>(null)
  const domains = useRoutingDomains()

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
        actions={canCreate && <Button variant="primary" onClick={() => setCreating(true)}><Plus size={16} aria-hidden /> New subnet</Button>}
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
          <Select aria-label="Routing domain" className="w-48!" value={routingDomainId} onChange={(e) => setFilter('routingDomainId', e.target.value)}>
            <option value="">All routing domains</option>
            {domains.data?.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
          </Select>
        </div>
        {isPending && <SkeletonRows rows={8} />}
        {error && <ErrorState error={error} onRetry={refetch} />}
        {data && data.data.length === 0 && (
          filtered
            ? <EmptyState title="No subnets match these filters" />
            : <EmptyState title="No subnets yet" body="Create a subnet to start tracking capacity and utilisation." />
        )}
        {data && data.data.length > 0 && (
          <>
            <div className="overflow-x-auto">
              <table className="w-full border-collapse">
                <thead>
                  <tr><Th>Name</Th><Th>CIDR</Th><Th>Routing domain</Th><Th>Environment</Th><Th>Gateway</Th><Th className="text-right">Usable</Th><Th>Utilisation</Th></tr>
                </thead>
                <tbody>
                  {data.data.map((s) => {
                    const pct = Number(s.utilisationPct)
                    return (
                      <tr key={s.id} className="hover:bg-surface-2">
                        <Td><button type="button" className="cursor-pointer text-left font-medium text-ink hover:text-accent" onClick={() => setSelected(s)}>{s.name}</button></Td>
                        <Td><Mono>{s.calculated.cidr}</Mono></Td>
                        <Td className="text-ink-2">{s.routingDomainName}</Td>
                        <Td><Badge tone={s.environment === 'production' ? 'accent' : 'neutral'}>{environments[s.environment]}</Badge></Td>
                        <Td className="text-ink-2">{s.gateway ? <Mono>{s.gateway}</Mono> : '—'}</Td>
                        <Td className="text-right text-ink-2 tabular-nums">{formatCapacity(s.calculated.usableAddresses, s.calculated.family, s.calculated.prefixLength)}</Td>
                        <Td><Meter pct={pct} level={s.utilisationLevel} label={`${s.name} utilisation`} /></Td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
            <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
          </>
        )}
      </Card>
      <CreateModal open={creating} onClose={() => setCreating(false)} />
      <DetailModal subnet={selected} onClose={() => setSelected(null)} />
    </>
  )
}
