import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { Button, Field, FormError, Input, Modal, Select, useConfirmSave } from '../../components/ui'
import { get, post, put } from '../../lib/api'
import { endOfDayDate, isoDate } from '../../lib/format'
import { applyServerErrors } from '../../lib/forms'
import { ipTypes, options } from '../../lib/labels'
import { inventoryKeys, useInvalidate, useSegments } from '../../lib/queries'
import { useCan } from '../../lib/session'
import type { Reservation, User } from '../../lib/types'
import { choose, futureDate, inNetwork, ipToInt, ipv4, optionalIPv4, requiredText, text } from '../../lib/validation'

const schema = z.object({
  segmentId: choose('a segment'), startIp: ipv4, endIp: optionalIPv4, reason: requiredText('Reason', 300),
  ownerId: z.string(), ownerName: text('Owner', 120), expiresOn: futureDate('Expiry'),
}).superRefine((v, ctx) => {
  if (v.endIp && ipToInt(v.endIp) < ipToInt(v.startIp)) ctx.addIssue({ code: 'custom', path: ['endIp'], message: 'The last IP must not be before the first.' })
  else if (v.endIp && ipToInt(v.endIp) - ipToInt(v.startIp) >= 256) ctx.addIssue({ code: 'custom', path: ['endIp'], message: 'Reserve at most 256 addresses at a time.' })
})
type Values = z.infer<typeof schema>
const blank: Values = { segmentId: '', startIp: '', endIp: '', reason: '', ownerId: '', ownerName: '', expiresOn: '' }

export function NewReservation({ open, onClose }: { open: boolean; onClose: () => void }) {
  const segments = useSegments()
  const isAdmin = useCan('inventory:edit')
  const users = useQuery({ queryKey: ['/users/options'], queryFn: () => get<User[]>('/users/options'), enabled: open && isAdmin })
  const refresh = useInvalidate(...inventoryKeys)
  const [error, setFormError] = useState<unknown>(null)
  const { register, handleSubmit, reset, setError, watch, formState: { errors } } = useForm<Values>({ resolver: zodResolver(schema), defaultValues: blank })
  useEffect(() => { if (open) { reset(blank); setFormError(null) } }, [open, reset])
  const segment = segments.data?.find((s) => s.id === watch('segmentId'))
  const save = useMutation({
    mutationFn: (v: Values) => post('/reservations', v),
    meta: { inlineError: true, success: 'Reservation created.' },
    onSuccess: () => { void refresh(); onClose() },
    onError: (e) => { if (!applyServerErrors(e, setError, Object.keys(blank))) setFormError(e) },
  })
  const submit = handleSubmit((v) => {
    for (const k of ['startIp', 'endIp'] as const) {
      if (segment && v[k] && !inNetwork(v[k], segment.network)) { setError(k, { message: `Must be inside ${segment.network}.` }); return }
    }
    save.mutate(v)
  })
  return (
    <Modal open={open} onClose={onClose} title="Reserve addresses" size="lg" subtitle="Hold an IP or a small range for a purpose. Nothing is released automatically when it expires."
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => void submit()}>Reserve</Button></>}>
      <form onSubmit={(e) => { e.preventDefault(); void submit() }} noValidate className="grid gap-3 sm:grid-cols-2">
        <div className="sm:col-span-2"><Field label="Segment" htmlFor="rs-seg" error={errors.segmentId?.message}>
          <Select id="rs-seg" {...register('segmentId')}><option value="">Choose a segment</option>{segments.data?.map((s) => <option key={s.id} value={s.id}>{s.name} · {s.network}</option>)}</Select>
        </Field></div>
        <Field label="First IP" htmlFor="rs-start" error={errors.startIp?.message}><Input id="rs-start" className="font-mono" {...register('startIp')} /></Field>
        <Field label="Last IP (optional)" htmlFor="rs-end" error={errors.endIp?.message} hint="Leave empty to hold one address."><Input id="rs-end" className="font-mono" {...register('endIp')} /></Field>
        <div className="sm:col-span-2"><Field label="Reason" htmlFor="rs-reason" error={errors.reason?.message}><Input id="rs-reason" placeholder="New branch printers" {...register('reason')} /></Field></div>
        {isAdmin && (
          <Field label="Owner" htmlFor="rs-owner" hint="Who extends, converts or releases it.">
            <Select id="rs-owner" {...register('ownerId')}><option value="">Me</option>{users.data?.map((u) => <option key={u.id} value={u.id}>{u.displayName || u.email}</option>)}</Select>
          </Field>
        )}
        <Field label="Expires (optional)" htmlFor="rs-exp" error={errors.expiresOn?.message} hint="The owner is reminded beforehand."><Input id="rs-exp" type="date" min={isoDate()} {...register('expiresOn')} /></Field>
        <div className="sm:col-span-2"><FormError error={error} /></div>
      </form>
    </Modal>
  )
}

/** Extends (or clears) the expiry and edits the reason. */
export function EditReservation({ hold, onClose }: { hold: Reservation | null; onClose: () => void }) {
  const [reason, setReason] = useState('')
  const [expiresOn, setExpires] = useState('')
  const refresh = useInvalidate('/reservations', '/dashboard')
  const confirmSave = useConfirmSave()
  useEffect(() => { if (hold) { setReason(hold.reason); setExpires(hold.expiresAt ? endOfDayDate(hold.expiresAt) : '') } }, [hold])
  const save = useMutation({
    mutationFn: () => put(`/reservations/${hold!.id}`, { reason, ownerName: hold!.ownerName, expiresOn }),
    meta: { inlineError: true, success: 'Reservation updated.' }, onSuccess: () => { void refresh(); onClose() },
  })
  useEffect(() => { save.reset() }, [hold]) // a dialog opened for another record starts without the last error
  const past = expiresOn !== '' && expiresOn < isoDate()
  return (
    <Modal open={hold !== null} onClose={onClose} title="Extend or edit reservation" subtitle={hold ? `${hold.startIp}${hold.endIp !== hold.startIp ? ` – ${hold.endIp}` : ''}` : undefined}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} disabled={!reason.trim() || past} onClick={() => void confirmSave('this reservation', () => save.mutate())}>Save changes</Button></>}>
      <div className="space-y-3">
        <Field label="Reason" htmlFor="er-reason" error={!reason.trim() ? 'Reason is required.' : undefined}><Input id="er-reason" value={reason} maxLength={300} onChange={(e) => setReason(e.target.value)} /></Field>
        <Field label="Expires" htmlFor="er-exp" error={past ? "Expiry can't be in the past." : undefined} hint="Clear the date to keep it with no expiry.">
          <div className="flex gap-2">
            <Input id="er-exp" type="date" min={isoDate()} value={expiresOn} onChange={(e) => setExpires(e.target.value)} />
            {[30, 90].map((d) => <Button key={d} onClick={() => setExpires(isoDate(d))}>+{d} days</Button>)}
          </div>
        </Field>
        <FormError error={save.error} />
      </div>
    </Modal>
  )
}

/** Turns one held address into a register entry; the rest of the range stays held. */
export function ConvertReservation({ hold, onClose }: { hold: Reservation | null; onClose: () => void }) {
  const [ip, setIp] = useState('')
  const [name, setName] = useState('')
  const [ipType, setType] = useState('other')
  const refresh = useInvalidate(...inventoryKeys)
  useEffect(() => { if (hold) { setIp(hold.startIp); setName(hold.reason); setType('other') } }, [hold])
  const inside = hold && ipv4.safeParse(ip).success && ipToInt(ip) >= ipToInt(hold.startIp) && ipToInt(ip) <= ipToInt(hold.endIp)
  const save = useMutation({
    mutationFn: () => post(`/reservations/${hold!.id}/convert`, { ip, name, ipType }),
    meta: { inlineError: true, success: () => `${ip} is now assigned in the register.` }, onSuccess: () => { void refresh(); onClose() },
  })
  useEffect(() => { save.reset() }, [hold]) // a dialog opened for another record starts without the last error
  return (
    <Modal open={hold !== null} onClose={onClose} title="Assign a reserved address" subtitle={hold ? `${hold.reason} · ${hold.size} address${hold.size > 1 ? 'es' : ''}` : undefined}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} disabled={!inside || !name.trim()} onClick={() => save.mutate()}>Assign {ip}</Button></>}>
      <div className="space-y-3">
        <Field label="Address" htmlFor="cv-ip" error={ip && !inside ? `Choose an address from ${hold?.startIp} to ${hold?.endIp}.` : undefined}
          hint={hold && hold.size > 1 ? 'The other addresses stay reserved.' : undefined}>
          <Input id="cv-ip" className="font-mono" value={ip} onChange={(e) => setIp(e.target.value)} />
        </Field>
        <Field label="Assigned to" htmlFor="cv-name"><Input id="cv-name" value={name} maxLength={120} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="Type" htmlFor="cv-type"><Select id="cv-type" value={ipType} onChange={(e) => setType(e.target.value)}>{options(ipTypes).map((o) => <option key={o.value} value={o.value}>{o.text}</option>)}</Select></Field>
        <FormError error={save.error} />
      </div>
    </Modal>
  )
}
