import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { Button, Field, FormError, Input, Modal, Select, Textarea, useConfirmSave } from '../../components/ui'
import { post, put } from '../../lib/api'
import { applyServerErrors } from '../../lib/forms'
import { deviceTypes, options } from '../../lib/labels'
import { inventoryKeys, useInvalidate } from '../../lib/queries'
import type { Device } from '../../lib/types'
import { platformOf } from '../../lib/platforms'
import { isMAC, requiredText, text } from '../../lib/validation'
import { DepartmentSelect } from '../shared/DepartmentSelect'
import { PlatformSelect } from '../shared/PlatformFields'

const schema = z.object({
  mac: z.string().trim(),
  name: requiredText('Name', 120),
  deviceType: z.string(),
  platform: z.string(),
  owner: text('Owner', 120),
  department: text('Department', 120),
  notes: text('Notes', 1000),
})
type Values = z.infer<typeof schema>

/** Adds a known device (approved straight away) or edits one. The MAC can't change. */
export function DeviceFormModal({ open, device, onClose }: { open: boolean; device?: Device; onClose: () => void }) {
  const refresh = useInvalidate(...inventoryKeys)
  const confirmSave = useConfirmSave()
  const [error, setFormError] = useState<unknown>(null)
  const { register, handleSubmit, reset, setError, watch, setValue, formState: { errors } } = useForm<Values>({ resolver: zodResolver(schema) })
  useEffect(() => {
    if (open) {
      reset({ mac: device?.mac ?? '', name: device?.name ?? '', deviceType: device?.deviceType ?? 'pc', platform: device?.platform ?? '', owner: device?.owner ?? '', department: device?.department ?? '', notes: device?.notes ?? '' })
      setFormError(null)
    }
  }, [open, device, reset])
  const save = useMutation({
    mutationFn: (v: Values) => (device ? put(`/devices/${device.id}`, v) : post('/devices', v)),
    meta: { inlineError: true, success: device ? 'Device saved.' : 'Device added and approved.' },
    onSuccess: () => { void refresh(); onClose() },
    onError: (e) => { if (!applyServerErrors(e, setError, ['mac', 'name', 'owner', 'department', 'notes'])) setFormError(e) },
  })
  const submit = handleSubmit((v) => {
    if (!device && !isMAC(v.mac)) {
      setError('mac', { message: 'Enter a MAC address such as 00:11:22:33:44:55.' })
      return
    }
    if (device) void confirmSave(device.name || device.mac, () => save.mutate(v))
    else save.mutate(v)
  })
  return (
    <Modal open={open} onClose={onClose} title={device ? 'Edit device' : 'Add a known device'} subtitle={device ? <span className="font-mono">{device.mac}</span> : 'Devices added here are approved. New MACs found by scans wait in Pending.'}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => void submit()}>{device ? 'Save changes' : 'Add device'}</Button></>}>
      <form onSubmit={(e) => { e.preventDefault(); void submit() }} noValidate className="grid gap-3 sm:grid-cols-2">
        {!device && <Field label="MAC address" htmlFor="d-mac" error={errors.mac?.message}><Input id="d-mac" className="font-mono" placeholder="00:11:22:33:44:55" {...register('mac')} /></Field>}
        <Field label="Name" htmlFor="d-name" error={errors.name?.message}><Input id="d-name" {...register('name')} /></Field>
        <Field label="Type" htmlFor="d-type"><Select id="d-type" {...register('deviceType')}>{options(deviceTypes).map((o) => <option key={o.value} value={o.value}>{o.text}</option>)}</Select></Field>
        <PlatformSelect id="d-platform" value={watch('platform')} onChange={(v) => setValue('platform', v)}
          guess={device ? platformOf(null, device.vendor, device.hostname)?.platform : undefined} />
        <Field label="Owner" htmlFor="d-owner" error={errors.owner?.message}><Input id="d-owner" {...register('owner')} /></Field>
        <DepartmentSelect id="d-dept" value={watch('department')} onChange={(v) => setValue('department', v, { shouldDirty: true })} error={errors.department?.message} />
        <div className="sm:col-span-2"><Field label="Notes" htmlFor="d-notes" error={errors.notes?.message}><Textarea id="d-notes" {...register('notes')} /></Field></div>
        <div className="sm:col-span-2"><FormError error={error} /></div>
      </form>
    </Modal>
  )
}
