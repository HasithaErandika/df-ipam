import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { Button, Field, FormError, FormSection, Input, Modal, Select, Switch, Textarea, useConfirmSave } from '../../components/ui'
import { post, put } from '../../lib/api'
import { applyServerErrors } from '../../lib/forms'
import { inventoryKeys, useEnvironments, useInvalidate } from '../../lib/queries'
import type { Segment } from '../../lib/types'
import { cidrIssue, inNetwork, ipToInt, optionalInt, optionalIPv4, requiredText, text, toInt } from '../../lib/validation'

const schema = z.object({
  name: requiredText('Name', 80),
  kind: z.enum(['dhcp', 'server', 'cloud']),
  floor: optionalInt('Floor', -5, 200),
  vlan: optionalInt('VLAN', 1, 4094),
  network: z.string().trim().superRefine((v, ctx) => { const issue = cidrIssue(v); if (issue) ctx.addIssue({ code: 'custom', message: issue }) }),
  gateway: optionalIPv4,
  dhcpStart: optionalIPv4,
  dhcpEnd: optionalIPv4,
  defaultEnvironmentId: z.string(),
  description: text('Description', 500),
  alertPrivateMacs: z.boolean(),
}).superRefine((v, ctx) => {
  const ok = !cidrIssue(v.network)
  for (const k of ['gateway', 'dhcpStart', 'dhcpEnd'] as const) {
    if (ok && v[k] && !inNetwork(v[k], v.network)) ctx.addIssue({ code: 'custom', path: [k], message: `Must be inside ${v.network}.` })
  }
  if (v.kind === 'dhcp' && Boolean(v.dhcpStart) !== Boolean(v.dhcpEnd)) ctx.addIssue({ code: 'custom', path: ['dhcpEnd'], message: 'Enter both the first and last pool address.' })
  if (v.dhcpStart && v.dhcpEnd && ipToInt(v.dhcpEnd) < ipToInt(v.dhcpStart)) ctx.addIssue({ code: 'custom', path: ['dhcpEnd'], message: 'The last pool address must not be before the first.' })
})
type Values = z.infer<typeof schema>
const fields = Object.keys(schema.shape)

const initial = (s?: Segment): Values => ({
  name: s?.name ?? '', kind: s?.kind ?? 'dhcp', floor: s?.floor?.toString() ?? '', vlan: s?.vlan?.toString() ?? '', network: s?.network ?? '',
  gateway: s?.gateway ?? '', dhcpStart: s?.dhcpStart ?? '', dhcpEnd: s?.dhcpEnd ?? '', defaultEnvironmentId: s?.defaultEnvironmentId ?? '', description: s?.description ?? '',
  alertPrivateMacs: s?.alertPrivateMacs ?? false,
})

const kindHelp = {
  dhcp: 'A floor network served by DHCP. Addresses outside the pool count as static.',
  server: 'Static server addresses. Entries take the default environment.',
  cloud: 'A network hosted in a cloud. Static, like servers, and never ping-swept from the building.',
}

export function SegmentForm({ open, segment, onClose }: { open: boolean; segment?: Segment; onClose: () => void }) {
  const environments = useEnvironments()
  const refresh = useInvalidate(...inventoryKeys)
  const confirmSave = useConfirmSave()
  const [error, setFormError] = useState<unknown>(null)
  const { register, handleSubmit, reset, watch, setValue, setError, formState: { errors } } = useForm<Values>({ resolver: zodResolver(schema), defaultValues: initial(segment) })
  useEffect(() => { if (open) { reset(initial(segment)); setFormError(null) } }, [open, segment, reset])
  const kind = watch('kind')
  const save = useMutation({
    mutationFn: (v: Values) => {
      const body = { ...v, floor: toInt(v.floor), vlan: toInt(v.vlan), dhcpStart: kind === 'dhcp' ? v.dhcpStart : '', dhcpEnd: kind === 'dhcp' ? v.dhcpEnd : '', defaultEnvironmentId: kind === 'dhcp' ? '' : v.defaultEnvironmentId }
      return segment ? put(`/segments/${segment.id}`, body) : post('/segments', body)
    },
    meta: { inlineError: true, success: (v: Values) => (segment ? `Saved ${v.name}.` : `${v.name} added. It is scanned on the next run.`) },
    onSuccess: () => { void refresh(); onClose() },
    onError: (e) => { if (!applyServerErrors(e, setError, fields)) setFormError(e) },
  })
  const submit = handleSubmit((v) => (segment ? void confirmSave(segment.name, () => save.mutate(v)) : save.mutate(v)))
  return (
    <Modal open={open} onClose={onClose} title={segment ? `Edit ${segment.name}` : 'Add a segment'} size="lg"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => void submit()}>{segment ? 'Save changes' : 'Add segment'}</Button></>}>
      <form onSubmit={(e) => { e.preventDefault(); void submit() }} noValidate className="space-y-5">
        <FormSection title="Segment">
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Name" htmlFor="sg-name" error={errors.name?.message}><Input id="sg-name" placeholder="Floor 4" {...register('name')} /></Field>
            <Field label="Type" htmlFor="sg-kind" hint={kindHelp[kind]}>
              <Select id="sg-kind" {...register('kind')}><option value="dhcp">Floor (DHCP)</option><option value="server">Server</option><option value="cloud">Cloud</option></Select>
            </Field>
            <Field label="Network" htmlFor="sg-net" error={errors.network?.message} hint="IPv4, /16 to /30. Networks can't overlap.">
              <Input id="sg-net" className="font-mono" placeholder="192.168.4.0/24" {...register('network')} />
            </Field>
            <Field label="Gateway (optional)" htmlFor="sg-gw" error={errors.gateway?.message}><Input id="sg-gw" className="font-mono" placeholder="192.168.4.1" {...register('gateway')} /></Field>
            {kind !== 'cloud' && <Field label="Floor (optional)" htmlFor="sg-floor" error={errors.floor?.message}><Input id="sg-floor" inputMode="numeric" {...register('floor')} /></Field>}
            <Field label="VLAN (optional)" htmlFor="sg-vlan" error={errors.vlan?.message}><Input id="sg-vlan" inputMode="numeric" {...register('vlan')} /></Field>
          </div>
        </FormSection>
        {kind === 'dhcp' ? (
          <FormSection title="DHCP pool">
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="First address" htmlFor="sg-ps" error={errors.dhcpStart?.message}><Input id="sg-ps" className="font-mono" placeholder="192.168.4.100" {...register('dhcpStart')} /></Field>
              <Field label="Last address" htmlFor="sg-pe" error={errors.dhcpEnd?.message}><Input id="sg-pe" className="font-mono" placeholder="192.168.4.250" {...register('dhcpEnd')} /></Field>
            </div>
            <p className="text-[12.5px] text-ink-3">Static detection depends on this range: anything in use outside it is treated as a hand-set IP.</p>
          </FormSection>
        ) : (
          <Field label="Default environment" htmlFor="sg-env" hint="New entries in this segment start with it. Each entry can still differ.">
            <Select id="sg-env" {...register('defaultEnvironmentId')}><option value="">None</option>{environments.data?.map((e) => <option key={e.id} value={e.id}>{e.name}</option>)}</Select>
          </Field>
        )}
        <FormSection title="Alerts">
          <Switch id="sg-private" checked={watch('alertPrivateMacs')} onChange={(v) => setValue('alertPrivateMacs', v)}
            label="Alert on devices with a private MAC"
            hint="Phones and laptops use random MACs on Wi-Fi, so these are not unknown-device alerts. Turn this on where every device matters: one alert per unregistered address, Medium on floors and High on server and cloud networks." />
        </FormSection>
        <Field label="Description" htmlFor="sg-desc" error={errors.description?.message}><Textarea id="sg-desc" {...register('description')} /></Field>
        <FormError error={error} />
      </form>
    </Modal>
  )
}
