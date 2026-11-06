import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { RoutingDomainSelect, SearchSelect } from '../../components/SearchSelect'
import { Button, Field, FormError, Input, Modal, MoreOptions, Select } from '../../components/ui'
import { useDebounced } from '../../components/workflow'
import { ApiError, listAll, post } from '../../lib/api'
import { environments } from '../../lib/format'
import type { NetInfo, Subnet } from '../../lib/types'
import { isCIDR, isIP } from '../../lib/validation'
import { Calculated } from './Calculated'
import { useOptions, usePeople } from './options'

const schema = z.object({
  name: z.string().trim().min(1, 'Enter a subnet name.'),
  cidr: z.string().trim().min(1, 'Enter a CIDR.').refine(isCIDR, 'Enter a network such as 10.20.30.0/24.'),
  routingDomainId: z.string().min(1, 'Choose a routing domain.'),
  environment: z.string().min(1, 'Choose an environment.'),
  gateway: z.string().trim().refine((v) => v === '' || isIP(v), 'Enter a valid IPv4 or IPv6 address.'),
  securityZone: z.string().trim(),
  description: z.string().trim(),
  siteId: z.string(),
  vlanId: z.string(),
  businessServiceId: z.string(),
  networkBlockId: z.string(),
  networkZoneId: z.string(),
  vdomId: z.string(),
  businessUnitId: z.string(),
  technicalOwnerId: z.string(),
  businessOwnerId: z.string(),
  overlapExceptionId: z.string(),
})

type FormValues = z.infer<typeof schema>

interface Preview { info: NetInfo; conflicts: { id: string; name: string; cidr: string }[] }

export function CreateModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const queryClient = useQueryClient()
  const sites = useOptions('/sites', open)
  const vlans = useOptions('/vlans', open)
  const services = useOptions('/business-services', open)
  const blocks = useOptions('/network-blocks', open)
  const zones = useOptions('/network-zones', open)
  const vdoms = useOptions('/vdoms', open)
  const units = useOptions('/business-units', open)
  const people = usePeople(open)
  const exceptions = useQuery({ queryKey: ['options', '/overlap-exceptions'], enabled: open, queryFn: () => listAll<{ id: string; reason: string }>('/overlap-exceptions') })
  const { register, control, handleSubmit, watch, reset, setError, formState: { errors } } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '', cidr: '', routingDomainId: '', environment: '', gateway: '', securityZone: '', description: '', siteId: '', vlanId: '', businessServiceId: '', networkBlockId: '', networkZoneId: '', vdomId: '', businessUnitId: '', technicalOwnerId: '', businessOwnerId: '', overlapExceptionId: '' },
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

  const create = useMutation({ meta: { success: 'Subnet created.', inlineError: true },
    mutationFn: (v: FormValues) => post<Subnet>('/subnets', {
      ...v,
      gateway: v.gateway || null,
      siteId: v.siteId || null,
      vlanId: v.vlanId || null,
      businessServiceId: v.businessServiceId || null,
      networkBlockId: v.networkBlockId || null,
      networkZoneId: v.networkZoneId || null,
      vdomId: v.vdomId || null,
      businessUnitId: v.businessUnitId || null,
      technicalOwnerId: v.technicalOwnerId || null,
      businessOwnerId: v.businessOwnerId || null,
      overlapExceptionId: v.overlapExceptionId || null,
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
            <Controller control={control} name="routingDomainId" render={({ field }) => <RoutingDomainSelect id="routingDomainId" value={field.value} onChange={field.onChange} />} />
          </Field>
          <MoreOptions label="More details (site, VLAN, owners)" forceOpen={Boolean(errors.siteId || errors.vlanId || errors.networkBlockId || errors.networkZoneId || errors.vdomId)}>
          <div className="grid grid-cols-2 gap-3">
            <Field label="Site (optional)" htmlFor="siteId" error={errors.siteId?.message}>
              <Controller control={control} name="siteId" render={({ field }) => <SearchSelect id="siteId" value={field.value} onChange={field.onChange} emptyLabel="None" loading={sites.isPending} fetchError={sites.error} onRetry={() => sites.refetch()} options={sites.data?.map((o) => ({ value: o.id, label: o.name })) ?? []} />} />
            </Field>
            <Field label="VLAN (optional)" htmlFor="vlanId" error={errors.vlanId?.message}>
              <Controller control={control} name="vlanId" render={({ field }) => <SearchSelect id="vlanId" value={field.value} onChange={field.onChange} emptyLabel="None" loading={vlans.isPending} fetchError={vlans.error} onRetry={() => vlans.refetch()} options={vlans.data?.map((o) => ({ value: o.id, label: `${o.vlanId} ${o.name}` })) ?? []} />} />
            </Field>
            <Field label="Business service (optional)" htmlFor="businessServiceId">
              <Controller control={control} name="businessServiceId" render={({ field }) => <SearchSelect id="businessServiceId" value={field.value} onChange={field.onChange} emptyLabel="None" loading={services.isPending} fetchError={services.error} onRetry={() => services.refetch()} options={services.data?.map((o) => ({ value: o.id, label: o.name })) ?? []} />} />
            </Field>
            <Field label="Network block (optional)" htmlFor="networkBlockId" error={errors.networkBlockId?.message}>
              <Controller control={control} name="networkBlockId" render={({ field }) => <SearchSelect id="networkBlockId" value={field.value} onChange={field.onChange} emptyLabel="None" loading={blocks.isPending} fetchError={blocks.error} onRetry={() => blocks.refetch()} options={blocks.data?.map((o) => ({ value: o.id, label: `${o.cidr} ${o.name}` })) ?? []} />} />
            </Field>
            <Field label="Network zone (optional)" htmlFor="networkZoneId" error={errors.networkZoneId?.message}>
              <Controller control={control} name="networkZoneId" render={({ field }) => <SearchSelect id="networkZoneId" value={field.value} onChange={field.onChange} emptyLabel="None" loading={zones.isPending} fetchError={zones.error} onRetry={() => zones.refetch()} options={zones.data?.map((o) => ({ value: o.id, label: o.name })) ?? []} />} />
            </Field>
            <Field label="VDOM (optional)" htmlFor="vdomId" error={errors.vdomId?.message}>
              <Controller control={control} name="vdomId" render={({ field }) => <SearchSelect id="vdomId" value={field.value} onChange={field.onChange} emptyLabel="None" loading={vdoms.isPending} fetchError={vdoms.error} onRetry={() => vdoms.refetch()} options={vdoms.data?.map((o) => ({ value: o.id, label: o.name })) ?? []} />} />
            </Field>
            <Field label="Business unit (optional)" htmlFor="businessUnitId">
              <Controller control={control} name="businessUnitId" render={({ field }) => <SearchSelect id="businessUnitId" value={field.value} onChange={field.onChange} emptyLabel="None" loading={units.isPending} fetchError={units.error} onRetry={() => units.refetch()} options={units.data?.map((o) => ({ value: o.id, label: o.name })) ?? []} />} />
            </Field>
            <Field label="Technical owner (optional)" htmlFor="technicalOwnerId">
              <Controller control={control} name="technicalOwnerId" render={({ field }) => <SearchSelect id="technicalOwnerId" value={field.value} onChange={field.onChange} emptyLabel="None" loading={people.isPending} fetchError={people.error} onRetry={() => people.refetch()} options={people.data?.map((o) => ({ value: o.id, label: o.email })) ?? []} />} />
            </Field>
            <Field label="Business owner (optional)" htmlFor="businessOwnerId">
              <Controller control={control} name="businessOwnerId" render={({ field }) => <SearchSelect id="businessOwnerId" value={field.value} onChange={field.onChange} emptyLabel="None" loading={people.isPending} fetchError={people.error} onRetry={() => people.refetch()} options={people.data?.map((o) => ({ value: o.id, label: o.email })) ?? []} />} />
            </Field>
            <Field label="Security zone (optional)" htmlFor="securityZone"><Input id="securityZone" {...register('securityZone')} /></Field>
            <Field label="Description (optional)" htmlFor="description"><Input id="description" {...register('description')} /></Field>
            {(exceptions.data?.length ?? 0) > 0 && (
              <Field label="Overlap exception (optional)" htmlFor="overlapExceptionId" hint="Only for an approved, documented overlap.">
                <Controller control={control} name="overlapExceptionId" render={({ field }) => <SearchSelect id="overlapExceptionId" value={field.value} onChange={field.onChange} emptyLabel="None" loading={exceptions.isPending} fetchError={exceptions.error} onRetry={() => exceptions.refetch()} options={exceptions.data?.map((o) => ({ value: o.id, label: o.reason })) ?? []} />} />
              </Field>
            )}
          </div>
          </MoreOptions>
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
