import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation } from '@tanstack/react-query'
import { Sparkles } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { Button, Field, FormError, FormSection, Input, Modal, Select, Switch, Textarea, useConfirmSave } from '../../components/ui'
import { get, post, put, queryString } from '../../lib/api'
import { applyServerErrors } from '../../lib/forms'
import { ipTypes, options, registerStatuses } from '../../lib/labels'
import { inventoryKeys, useEnvironments, useInvalidate, useSegments } from '../../lib/queries'
import type { RegisterEntry } from '../../lib/types'
import { choose, inNetwork, ipv4, optionalInt, optionalMAC, text, toInt } from '../../lib/validation'
import { AddressCheck } from './AddressCheck'
import { DepartmentSelect } from './DepartmentSelect'
import { PlatformSelect, ServicePicker } from './PlatformFields'

const schema = z.object({
  segmentId: choose('a segment'),
  ip: ipv4,
  status: z.enum(['assigned', 'reserved', 'blocked']),
  ipType: z.string(),
  environmentId: z.string(),
  assignedName: text('Assigned to', 120),
  platform: z.string(),
  services: z.array(z.string()).max(10),
  expectedMac: optionalMAC,
  owner: text('Owner', 120),
  department: text('Department', 120),
  monitor: z.boolean(),
  critical: z.boolean(),
  checkPort: optionalInt('Check port', 1, 65535),
  notes: text('Notes', 1000),
}).superRefine((v, ctx) => {
  if (v.status === 'assigned' && !v.assignedName) ctx.addIssue({ code: 'custom', path: ['assignedName'], message: 'Say who or what the address is assigned to.' })
  if (v.critical && !v.monitor) ctx.addIssue({ code: 'custom', path: ['critical'], message: 'Turn on Monitor to mark an entry critical.' })
})
type Values = z.infer<typeof schema>
const fields = Object.keys(schema.shape) as (keyof Values)[]

/** What to pre-fill when registering from something seen on the network. */
export interface RegisterPrefill { segmentId?: string; ip?: string; mac?: string | null; name?: string | null; ipType?: string; platform?: string; platformGuess?: string | null }

function initial(entry?: RegisterEntry, prefill?: RegisterPrefill): Values {
  return {
    segmentId: entry?.segmentId ?? prefill?.segmentId ?? '', ip: entry?.ip ?? prefill?.ip ?? '', status: entry?.status ?? 'assigned',
    ipType: entry?.ipType ?? prefill?.ipType ?? 'other', environmentId: entry?.environmentId ?? '', assignedName: entry?.assignedName ?? prefill?.name ?? '',
    platform: entry?.platform ?? prefill?.platform ?? '', services: entry?.services ?? [],
    expectedMac: entry?.expectedMac ?? prefill?.mac ?? '', owner: entry?.owner ?? '', department: entry?.department ?? '',
    monitor: entry?.monitor ?? false, critical: entry?.critical ?? false, checkPort: entry?.checkPort ? String(entry.checkPort) : '', notes: entry?.notes ?? '',
  }
}

/**
 * Adds an address to the IP register, or edits an entry. A new entry can take the
 * suggested Available candidate, and the environment sets Monitor and Critical
 * to that environment's defaults (they can still be changed by hand).
 */
export function RegisterIPModal({ open, onClose, entry, prefill, serverOnly }: {
  open: boolean; onClose: () => void; entry?: RegisterEntry; prefill?: RegisterPrefill; serverOnly?: boolean
}) {
  const segments = useSegments()
  const environments = useEnvironments()
  const refresh = useInvalidate(...inventoryKeys)
  const confirmSave = useConfirmSave()
  const [formError, setFormError] = useState<unknown>(null)
  const form = useForm<Values>({ resolver: zodResolver(schema), defaultValues: initial(entry, prefill) })
  const { register, handleSubmit, watch, setValue, reset, setError, formState: { errors } } = form
  useEffect(() => { if (open) { reset(initial(entry, prefill)); setFormError(null) } }, [open, entry, prefill, reset])

  const segmentId = watch('segmentId')
  const segment = segments.data?.find((s) => s.id === segmentId)
  const choices = (segments.data ?? []).filter((s) => !serverOnly || s.kind !== 'dhcp')

  // A new entry in a server segment starts with that segment's environment and its defaults.
  useEffect(() => {
    if (entry || !segment) return
    if (segment.kind !== 'dhcp' && !form.getValues('environmentId') && segment.defaultEnvironmentId) setValue('environmentId', segment.defaultEnvironmentId)
    if (segment.kind !== 'dhcp' && form.getValues('ipType') === 'other') setValue('ipType', 'server')
  }, [segment, entry, setValue, form])
  const environmentId = watch('environmentId')
  useEffect(() => {
    if (entry) return
    const env = environments.data?.find((e) => e.id === environmentId)
    if (env) { setValue('monitor', env.defaultMonitor); setValue('critical', env.defaultCritical) }
  }, [environmentId, environments.data, entry, setValue])

  const suggest = useMutation({
    mutationFn: () => get<{ ip: string }>(`/ip-addresses/available-candidate${queryString({ segmentId })}`),
    onSuccess: (r) => setValue('ip', r.ip, { shouldValidate: true }),
    meta: { inlineError: true },
    onError: setFormError,
  })

  const save = useMutation({
    mutationFn: (v: Values) => {
      const body = { ...v, checkPort: toInt(v.checkPort) }
      return entry ? put(`/ip-addresses/${entry.id}`, body) : post('/ip-addresses', body)
    },
    meta: { inlineError: true, success: (v: Values) => (entry ? `Saved ${v.ip}.` : `${v.ip} added to the register.`) },
    onSuccess: () => { void refresh(); onClose() },
    onError: (e) => { if (!applyServerErrors(e, setError, fields)) setFormError(e) },
  })

  const submit = handleSubmit((v) => {
    if (segment && !inNetwork(v.ip, segment.network)) {
      setError('ip', { message: `Must be inside ${segment.network}.` })
      return
    }
    setFormError(null)
    if (entry) void confirmSave(entry.ip, () => save.mutate(v))
    else save.mutate(v)
  })

  const monitor = watch('monitor')
  return (
    <Modal open={open} onClose={onClose} size="lg" title={entry ? `Edit ${entry.ip}` : 'Add to the IP register'}
      subtitle={entry ? entry.segmentName : 'The register is the official record. Set the IP on the machine yourself; DF-IPAM never changes the network.'}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => void submit()}>{entry ? 'Save changes' : 'Add to register'}</Button></>}>
      <form onSubmit={(e) => { e.preventDefault(); void submit() }} noValidate className="space-y-5">
        <FormSection title="Address">
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Segment" htmlFor="reg-segment" error={errors.segmentId?.message}>
              <Select id="reg-segment" disabled={Boolean(entry)} {...register('segmentId')}>
                <option value="">Choose a segment</option>
                {choices.map((s) => <option key={s.id} value={s.id}>{s.name} · {s.network}</option>)}
              </Select>
            </Field>
            <Field label="IP address" htmlFor="reg-ip" error={errors.ip?.message} hint={segment && !entry ? `Inside ${segment.network}` : undefined}>
              <div className="flex gap-2">
                <Input id="reg-ip" className="font-mono" disabled={Boolean(entry)} autoComplete="off" placeholder="192.168.1.10" {...register('ip')} />
                {!entry && <Button title="Suggest the next Available candidate" disabled={!segmentId} busy={suggest.isPending} onClick={() => suggest.mutate()}><Sparkles size={14} aria-hidden />Suggest</Button>}
              </div>
            </Field>
          </div>
          {!entry && <AddressCheck segmentId={segmentId} ip={watch('ip')} />}
          <div className="grid gap-3 sm:grid-cols-3">
            <Field label="Status" htmlFor="reg-status"><Select id="reg-status" {...register('status')}>{options(registerStatuses).map((o) => <option key={o.value} value={o.value}>{o.text}</option>)}</Select></Field>
            <Field label="Type" htmlFor="reg-type"><Select id="reg-type" {...register('ipType')}>{options(ipTypes).map((o) => <option key={o.value} value={o.value}>{o.text}</option>)}</Select></Field>
            <Field label="Environment" htmlFor="reg-env">
              <Select id="reg-env" {...register('environmentId')}>
                <option value="">None</option>
                {environments.data?.map((e) => <option key={e.id} value={e.id}>{e.name}</option>)}
              </Select>
            </Field>
          </div>
        </FormSection>
        <FormSection title="Assignment">
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Assigned to" htmlFor="reg-name" error={errors.assignedName?.message} hint="A device or a description, such as “HR printer, 3rd floor”.">
              <Input id="reg-name" {...register('assignedName')} />
            </Field>
            <Field label="Expected MAC (optional)" htmlFor="reg-mac" error={errors.expectedMac?.message} hint="An alert is raised if another MAC answers.">
              <Input id="reg-mac" className="font-mono" placeholder="00:11:22:33:44:55" {...register('expectedMac')} />
            </Field>
            <PlatformSelect id="reg-platform" value={watch('platform')} onChange={(v) => setValue('platform', v)} guess={entry ? undefined : prefill?.platformGuess} />
            <div className="hidden sm:block" />
            <Field label="Owner" htmlFor="reg-owner" error={errors.owner?.message}><Input id="reg-owner" {...register('owner')} /></Field>
            <DepartmentSelect id="reg-dept" value={watch('department')} onChange={(v) => setValue('department', v, { shouldDirty: true })} error={errors.department?.message} />
          </div>
        </FormSection>
        {(watch('ipType') === 'server' || segment?.kind !== 'dhcp') && (
          <ServicePicker value={watch('services')} onChange={(v) => setValue('services', v)} />
        )}
        <FormSection title="Monitoring">
          <Switch id="reg-monitor" checked={monitor} onChange={(v) => { setValue('monitor', v); if (!v) setValue('critical', false) }}
            label="Monitor this address" hint="Checked every minute by ping, or the TCP port below when ping is blocked." />
          <Switch id="reg-critical" checked={watch('critical')} disabled={!monitor} onChange={(v) => setValue('critical', v)}
            label="Critical" hint="Down or a MAC change raises a Critical alert, whatever the environment." />
          {errors.critical && <p role="alert" className="text-[13px] text-danger">{errors.critical.message}</p>}
          {monitor && (
            <Field label="Check port (optional)" htmlFor="reg-port" error={errors.checkPort?.message} hint="For example 443 or 3389.">
              <Input id="reg-port" inputMode="numeric" className="w-32" {...register('checkPort')} />
            </Field>
          )}
        </FormSection>
        <Field label="Notes" htmlFor="reg-notes" error={errors.notes?.message}><Textarea id="reg-notes" {...register('notes')} /></Field>
        <FormError error={formError} />
      </form>
    </Modal>
  )
}
