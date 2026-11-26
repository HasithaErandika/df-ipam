import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { Button, Field, FormError, Input, Modal, Select, Textarea } from '../../components/ui'
import { post } from '../../lib/api'
import { isoDate } from '../../lib/format'
import { applyServerErrors } from '../../lib/forms'
import { ipTypes, options } from '../../lib/labels'
import { useEnvironments, useInvalidate, useSegments } from '../../lib/queries'
import { choose, futureDate, inNetwork, optionalIPv4, requiredText } from '../../lib/validation'

const schema = z.object({
  segmentId: choose('a segment'),
  ipType: z.string(),
  environmentId: z.string(),
  name: requiredText('Device name', 120),
  purpose: requiredText('Purpose', 1000),
  neededBy: futureDate('Needed by'),
  requestedIp: optionalIPv4,
})
type Values = z.infer<typeof schema>
const empty: Values = { segmentId: '', ipType: 'server', environmentId: '', name: '', purpose: '', neededBy: '', requestedIp: '' }

/** Raises a request for an IP. An Admin approves it with one address, in one step. */
export function RequestForm({ open, onClose }: { open: boolean; onClose: () => void }) {
  const segments = useSegments()
  const environments = useEnvironments()
  const refresh = useInvalidate('/ip-requests', '/dashboard')
  const [error, setFormError] = useState<unknown>(null)
  const { register, handleSubmit, reset, setError, watch, formState: { errors } } = useForm<Values>({ resolver: zodResolver(schema), defaultValues: empty })
  useEffect(() => { if (open) { reset(empty); setFormError(null) } }, [open, reset])
  const segment = segments.data?.find((s) => s.id === watch('segmentId'))
  const create = useMutation({
    mutationFn: (v: Values) => post('/ip-requests', v),
    meta: { inlineError: true, success: 'Request sent. Admins have been notified.' },
    onSuccess: () => { void refresh(); onClose() },
    onError: (e) => { if (!applyServerErrors(e, setError, ['segmentId', 'requestedIp', 'name', 'purpose', 'neededBy'])) setFormError(e) },
  })
  const submit = handleSubmit((v) => {
    if (segment && v.requestedIp && !inNetwork(v.requestedIp, segment.network)) {
      setError('requestedIp', { message: `Must be inside ${segment.network}.` })
      return
    }
    create.mutate(v)
  })
  return (
    <Modal open={open} onClose={onClose} title="Request an IP" size="lg" subtitle="For a new server, printer or other device. You'll be told the address once it is approved."
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={create.isPending} onClick={() => void submit()}>Send request</Button></>}>
      <form onSubmit={(e) => { e.preventDefault(); void submit() }} noValidate className="grid gap-3 sm:grid-cols-2">
        <Field label="Segment" htmlFor="rq-seg" error={errors.segmentId?.message}>
          <Select id="rq-seg" {...register('segmentId')}><option value="">Choose a segment</option>{segments.data?.map((s) => <option key={s.id} value={s.id}>{s.name} · {s.network}</option>)}</Select>
        </Field>
        <Field label="Device type" htmlFor="rq-type"><Select id="rq-type" {...register('ipType')}>{options(ipTypes).map((o) => <option key={o.value} value={o.value}>{o.text}</option>)}</Select></Field>
        <Field label="Device name" htmlFor="rq-name" error={errors.name?.message}><Input id="rq-name" placeholder="app-server-03" {...register('name')} /></Field>
        <Field label="Environment" htmlFor="rq-env" hint="For servers.">
          <Select id="rq-env" {...register('environmentId')}><option value="">Segment default</option>{environments.data?.map((e) => <option key={e.id} value={e.id}>{e.name}</option>)}</Select>
        </Field>
        <div className="sm:col-span-2"><Field label="Purpose" htmlFor="rq-purpose" error={errors.purpose?.message}><Textarea id="rq-purpose" placeholder="What it is for, and who will use it" {...register('purpose')} /></Field></div>
        <Field label="Needed by (optional)" htmlFor="rq-date" error={errors.neededBy?.message}><Input id="rq-date" type="date" min={isoDate()} {...register('neededBy')} /></Field>
        <Field label="Preferred IP (optional)" htmlFor="rq-ip" error={errors.requestedIp?.message} hint="Leave empty to get the next Available candidate.">
          <Input id="rq-ip" className="font-mono" {...register('requestedIp')} />
        </Field>
        <div className="sm:col-span-2"><FormError error={error} /></div>
      </form>
    </Modal>
  )
}
