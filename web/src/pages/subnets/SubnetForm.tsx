import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { Button, ErrorState, Field, FormError, Input, Modal, Mono, Select, SkeletonRows } from '../../components/ui'
import { ApiError, listAll, patch, post } from '../../lib/api'
import { environments, formatCapacity } from '../../lib/format'
import { optionLabel, useDebounced, useOptions } from '../../lib/hooks'
import type { NetInfo, RoutingDomain, Subnet } from '../../lib/types'

export const useRoutingDomains = () =>
  useQuery({ queryKey: ['routing-domains', 'all'], queryFn: () => listAll<RoutingDomain>('/routing-domains') })

export function Detail({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex justify-between gap-4 border-b border-line py-1.5 last:border-0">
      <dt className="text-ink-2">{label}</dt>
      <dd className="text-right text-ink">{children}</dd>
    </div>
  )
}

export function Calculated({ info }: { info: NetInfo }) {
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

const threshold = z.string().regex(/^\d{1,3}$/, 'Enter 1 to 100.')

const schema = z.object({
  name: z.string().trim().min(1, 'Enter a subnet name.'),
  cidr: z.string().trim().min(1, 'Enter a CIDR.'),
  routingDomainId: z.string().min(1, 'Choose a routing domain.'),
  environment: z.string().min(1, 'Choose an environment.'),
  gateway: z.string().trim(),
  securityZone: z.string().trim(),
  description: z.string().trim(),
  sensitivity: z.string(),
  discoveryEnabled: z.boolean(),
  warnThreshold: threshold,
  criticalThreshold: threshold,
  siteId: z.string(),
  networkZoneId: z.string(),
  vdomId: z.string(),
  vlanId: z.string(),
  networkBlockId: z.string(),
  businessUnitId: z.string(),
  businessServiceId: z.string(),
  technicalOwnerId: z.string(),
  businessOwnerId: z.string(),
  overlapExceptionId: z.string(),
})
type FormValues = z.infer<typeof schema>

const mappingKeys = ['siteId', 'networkZoneId', 'vdomId', 'vlanId', 'networkBlockId', 'businessUnitId', 'businessServiceId', 'technicalOwnerId', 'businessOwnerId', 'overlapExceptionId'] as const

function defaults(s: Subnet | null): FormValues {
  return {
    name: s?.name ?? '', cidr: s?.calculated.cidr ?? '', routingDomainId: s?.routingDomainId ?? '', environment: s?.environment ?? '',
    gateway: s?.gateway ?? '', securityZone: s?.securityZone ?? '', description: s?.description ?? '', sensitivity: s?.sensitivity ?? 'standard',
    discoveryEnabled: s?.discoveryEnabled ?? false, warnThreshold: String(s?.warnThreshold ?? 80), criticalThreshold: String(s?.criticalThreshold ?? 95),
    siteId: s?.site?.id ?? '', networkZoneId: s?.networkZone?.id ?? '', vdomId: s?.vdom?.id ?? '', vlanId: s?.vlan?.id ?? '',
    networkBlockId: s?.networkBlock?.id ?? '', businessUnitId: s?.businessUnit?.id ?? '', businessServiceId: s?.businessService?.id ?? '',
    technicalOwnerId: s?.technicalOwner?.id ?? '', businessOwnerId: s?.businessOwner?.id ?? '', overlapExceptionId: s?.overlapExceptionId ?? '',
  }
}

interface Preview { info: NetInfo; conflicts: { id: string; name: string; cidr: string }[] }

export const sensitivities: Record<string, string> = { standard: 'Standard', sensitive: 'Sensitive', restricted: 'Restricted' }

type Options = Record<'sites' | 'zones' | 'vdoms' | 'vlans' | 'blocks' | 'units' | 'services' | 'owners' | 'exceptions', ReturnType<typeof useOptions>>

/**
 * Creates a subnet, or edits one with a version check. CIDR and routing domain are fixed once created.
 * The form waits for its dropdown options, so an existing mapping is shown, not "None".
 */
export function SubnetForm({ subnet, onClose, onSaved }: { subnet: Subnet | null; onClose: () => void; onSaved?: (s: Subnet) => void }) {
  const domains = useRoutingDomains()
  const opt: Options = {
    sites: useOptions('/sites'), zones: useOptions('/network-zones'), vdoms: useOptions('/vdoms'), vlans: useOptions('/vlans'),
    blocks: useOptions('/network-blocks'), units: useOptions('/business-units'), services: useOptions('/business-services'),
    owners: useOptions('/users/lookup'), exceptions: useOptions('/overlap-exceptions'),
  }
  const all = [domains, ...Object.values(opt)]
  const failed = all.find((q) => q.error)
  if (failed) {
    return <Modal open onClose={onClose} title={subnet ? `Edit ${subnet.name}` : 'New subnet'}><ErrorState error={failed.error} onRetry={() => all.forEach((q) => q.refetch())} /></Modal>
  }
  if (all.some((q) => q.isPending)) {
    return <Modal open onClose={onClose} title={subnet ? `Edit ${subnet.name}` : 'New subnet'} size="xl"><SkeletonRows rows={8} /></Modal>
  }
  return <Form subnet={subnet} onClose={onClose} onSaved={onSaved} domains={domains.data ?? []} opt={opt} />
}

function Form({ subnet, onClose, onSaved, domains: domainList, opt }:
  { subnet: Subnet | null; onClose: () => void; onSaved?: (s: Subnet) => void; domains: RoutingDomain[]; opt: Options }) {
  const qc = useQueryClient()
  const editing = subnet !== null
  const domains = { data: domainList }
  const { register, handleSubmit, watch, setError, formState: { errors } } = useForm<FormValues>({ resolver: zodResolver(schema), defaultValues: defaults(subnet) })
  const cidr = useDebounced(watch('cidr').trim())
  const routingDomainId = watch('routingDomainId')
  const preview = useQuery({
    queryKey: ['subnet-preview', cidr, routingDomainId],
    enabled: !editing && cidr.includes('/'),
    retry: false,
    queryFn: async (): Promise<Preview> => {
      const info = await post<NetInfo>('/subnets/calculate', { cidr })
      if (!routingDomainId) return { info, conflicts: [] }
      const { conflicts } = await post<{ conflicts: Preview['conflicts'] }>('/subnets/check-overlap', { cidr, routingDomainId })
      return { info, conflicts }
    },
  })
  const kind = domains.data?.find((d) => d.id === routingDomainId)?.kind

  const save = useMutation({
    mutationFn: (v: FormValues) => {
      const shared = {
        name: v.name, environment: v.environment, gateway: v.gateway, securityZone: v.securityZone, description: v.description,
        discoveryEnabled: v.discoveryEnabled, warnThreshold: Number(v.warnThreshold), criticalThreshold: Number(v.criticalThreshold),
      }
      if (editing) {
        const mappings = Object.fromEntries(mappingKeys.map((k) => [k, v[k]]))
        return patch<Subnet>(`/subnets/${subnet.id}`, { ...shared, sensitivity: v.sensitivity, ...mappings }, subnet.version)
      }
      const mappings = Object.fromEntries(mappingKeys.filter((k) => v[k]).map((k) => [k, v[k]]))
      return post<Subnet>('/subnets', { ...shared, cidr: v.cidr, routingDomainId: v.routingDomainId, gateway: v.gateway || null, ...mappings })
    },
    onSuccess: (s) => {
      qc.invalidateQueries({ queryKey: ['subnets'] })
      qc.invalidateQueries({ queryKey: ['summary'] })
      qc.invalidateQueries({ queryKey: ['catalog'] })
      onSaved?.(s)
      onClose()
    },
    onError: (e) => {
      if (e instanceof ApiError) for (const d of e.details) setError(d.field as keyof FormValues, { message: d.issue })
    },
  })
  const previewIssue = preview.error instanceof ApiError ? preview.error.details[0]?.issue : undefined
  const fieldLevel = save.error instanceof ApiError && save.error.details.length > 0 && save.error.details.some((d) => d.field in defaults(null))

  const select = (key: keyof FormValues, label: string, options: { id: string; label: string }[], hint?: string) => (
    <Field label={`${label} (optional)`} htmlFor={key} error={errors[key]?.message} hint={hint}>
      <Select id={key} {...register(key)}><option value="">None</option>{options.map((o) => <option key={o.id} value={o.id}>{o.label}</option>)}</Select>
    </Field>
  )
  const labelled = (q: ReturnType<typeof useOptions>) => (q.data ?? []).map((o) => ({ id: o.id, label: optionLabel(o) }))
  const exceptionOptions = (opt.exceptions.data ?? []).filter((o) => !o.expired || o.id === subnet?.overlapExceptionId).map((o) => ({ id: o.id, label: optionLabel(o) }))

  return (
    <Modal open onClose={onClose} size="xl" title={editing ? `Edit ${subnet.name}` : 'New subnet'} subtitle={editing ? <Mono>{subnet.calculated.cidr}</Mono> : undefined}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" type="submit" form="subnet-form" busy={save.isPending}>{editing ? 'Save changes' : 'Create subnet'}</Button></>}>
      <form id="subnet-form" noValidate onSubmit={handleSubmit((v) => save.mutate(v))} className={editing ? 'space-y-3.5' : 'grid gap-5 md:grid-cols-[1fr_17rem]'}>
        <div className="space-y-3.5">
          {!fieldLevel && <FormError error={save.error} />}
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Name" htmlFor="name" error={errors.name?.message}><Input id="name" autoFocus {...register('name')} /></Field>
            <Field label="Environment" htmlFor="environment" error={errors.environment?.message}>
              <Select id="environment" {...register('environment')}>
                <option value="">Select</option>
                {Object.entries(environments).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
              </Select>
            </Field>
            <Field label="CIDR" htmlFor="cidr" error={errors.cidr?.message ?? previewIssue} hint={editing ? "CIDR can't change once addresses depend on it." : undefined}>
              <Input id="cidr" className="font-mono" spellCheck={false} placeholder="10.20.30.0/24" disabled={editing} {...register('cidr')} />
            </Field>
            <Field label="Gateway (optional)" htmlFor="gateway" error={errors.gateway?.message}>
              <Input id="gateway" className="font-mono" spellCheck={false} {...register('gateway')} />
            </Field>
          </div>
          <Field label="Routing domain" htmlFor="routingDomainId" error={errors.routingDomainId?.message}
            hint={kind === 'vpc' || kind === 'vnet' ? "The provider's five reserved addresses are recorded automatically." : 'Overlaps are only blocked within the same routing domain.'}>
            <Select id="routingDomainId" disabled={editing} {...register('routingDomainId')}>
              <option value="">Select</option>
              {domains.data?.map((d) => <option key={d.id} value={d.id}>{d.organisationName} / {d.name}</option>)}
            </Select>
          </Field>
          <div className="grid gap-3 sm:grid-cols-2">
            {select('siteId', 'Site', labelled(opt.sites))}
            {select('networkZoneId', 'Network zone', labelled(opt.zones))}
            {select('vlanId', 'VLAN', labelled(opt.vlans))}
            {select('vdomId', 'VDOM', labelled(opt.vdoms))}
            {select('networkBlockId', 'Network block', labelled(opt.blocks))}
            {select('businessServiceId', 'Business service', labelled(opt.services))}
            {select('businessUnitId', 'Business unit', labelled(opt.units))}
            {select('overlapExceptionId', 'Overlap exception', exceptionOptions, 'Only with an approved reason to overlap.')}
            {select('technicalOwnerId', 'Technical owner', labelled(opt.owners))}
            {select('businessOwnerId', 'Business owner', labelled(opt.owners))}
            <Field label="Security zone (optional)" htmlFor="securityZone"><Input id="securityZone" {...register('securityZone')} /></Field>
            {editing && (
              <Field label="Sensitivity" htmlFor="sensitivity" error={errors.sensitivity?.message}>
                <Select id="sensitivity" {...register('sensitivity')}>{Object.entries(sensitivities).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</Select>
              </Field>
            )}
            <Field label="Warning at %" htmlFor="warnThreshold" error={errors.warnThreshold?.message}><Input id="warnThreshold" inputMode="numeric" {...register('warnThreshold')} /></Field>
            <Field label="Critical at %" htmlFor="criticalThreshold" error={errors.criticalThreshold?.message}><Input id="criticalThreshold" inputMode="numeric" {...register('criticalThreshold')} /></Field>
          </div>
          <Field label="Description (optional)" htmlFor="description"><Input id="description" {...register('description')} /></Field>
          <label className="flex items-center gap-2 text-ink">
            <input type="checkbox" className="size-4 accent-[var(--accent)]" {...register('discoveryEnabled')} />
            Include in discovery once approved scopes exist
          </label>
        </div>
        {!editing && (
          <aside className="self-start rounded-[5px] border border-line bg-surface-2 px-3 py-2">
            <h3 className="mb-1 text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase">Calculated</h3>
            {preview.data ? <Calculated info={preview.data.info} /> : <p className="py-2 text-[13px] text-ink-3">Enter a CIDR to see its range and capacity.</p>}
            {preview.data && preview.data.conflicts.length > 0 && (
              <p role="alert" className="mt-2 rounded-[4px] bg-danger-soft px-2 py-1.5 text-[12.5px] text-danger">
                Overlaps {preview.data.conflicts.map((c) => `${c.cidr} (${c.name})`).join(', ')}.
              </p>
            )}
          </aside>
        )}
      </form>
    </Modal>
  )
}
