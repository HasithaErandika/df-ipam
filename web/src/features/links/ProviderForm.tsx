import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { Button, Field, FormError, Input, Modal, Switch, Textarea, useConfirmSave } from '../../components/ui'
import { fieldIssues, post, put } from '../../lib/api'
import { listChanges } from '../../lib/changes'
import { useInvalidate } from '../../lib/queries'
import type { LinkProvider } from '../../lib/types'
import { isEmail } from '../../lib/validation'
import { linkKeys } from './queries'

type Values = Omit<LinkProvider, 'id' | 'links'>

const blank: Values = { name: '', accountManager: '', contactNumber: '', email: '', supportHotline: '', supportEmail: '', website: '', remarks: '', active: true }

const labels: Partial<Record<keyof Values, string>> = {
  name: 'Name', accountManager: 'Account manager', contactNumber: 'Contact number', email: 'Email', supportHotline: 'Support hotline',
  supportEmail: 'Support email', website: 'Website', remarks: 'Remarks', active: 'Active',
}

/** Adds a provider or edits one. The dialog is re-mounted for each provider. */
export function ProviderForm({ open, provider, onClose }: { open: boolean; provider?: LinkProvider; onClose: () => void }) {
  return open ? <ProviderDialog key={provider?.id ?? 'new'} provider={provider} onClose={onClose} /> : null
}

function ProviderDialog({ provider, onClose }: { provider?: LinkProvider; onClose: () => void }) {
  const start: Values = provider ? { ...provider } : blank
  const [v, setV] = useState<Values>(start)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const refresh = useInvalidate(...linkKeys)
  const confirmSave = useConfirmSave()
  const save = useMutation({
    mutationFn: () => (provider ? put(`/link-providers/${provider.id}`, v) : post('/link-providers', v)),
    meta: { inlineError: true, success: provider ? 'Provider saved.' : 'Provider added.' },
    onSuccess: () => { void refresh(); onClose() },
    onError: (e) => setErrors(fieldIssues(e)),
  })
  const submit = () => {
    const issues: Record<string, string> = {}
    if (!v.name.trim()) issues.name = 'Provider name is required.'
    for (const k of ['email', 'supportEmail'] as const) if (v[k].trim() && !isEmail(v[k].trim())) issues[k] = 'Enter an email address such as noc@provider.lk.'
    if (v.website.trim() && !/^https?:\/\//.test(v.website.trim())) issues.website = 'Website must start with https:// or http://.'
    setErrors(issues)
    if (Object.keys(issues).length) return
    if (provider) void confirmSave(provider.name, () => save.mutate(), undefined, listChanges(start, v, labels))
    else save.mutate()
  }
  const text = (k: Exclude<keyof Values, 'active'>, label: string, type?: string) => (
    <Field label={label} htmlFor={`prov-${k}`} error={errors[k]}>
      <Input id={`prov-${k}`} type={type} value={v[k]} onChange={(e) => setV({ ...v, [k]: e.target.value })} />
    </Field>
  )
  return (
    <Modal open onClose={onClose} size="lg" title={provider ? `Edit ${provider.name}` : 'Add a service provider'}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={submit}>{provider ? 'Save changes' : 'Add provider'}</Button></>}>
      <form noValidate onSubmit={(e) => { e.preventDefault(); submit() }} className="grid gap-3 sm:grid-cols-2">
        {text('name', 'Provider name *')}
        {text('website', 'Website', 'url')}
        {text('accountManager', 'Account manager')}
        {text('contactNumber', 'Contact number', 'tel')}
        {text('email', 'Email address', 'email')}
        {text('supportHotline', 'Support hotline', 'tel')}
        {text('supportEmail', 'Support email', 'email')}
        <div className="sm:col-span-2"><Field label="Remarks" htmlFor="prov-remarks" error={errors.remarks}><Textarea id="prov-remarks" value={v.remarks} onChange={(e) => setV({ ...v, remarks: e.target.value })} /></Field></div>
        {provider && <div className="sm:col-span-2"><Switch id="prov-active" checked={v.active} onChange={(a) => setV({ ...v, active: a })} label="Active" hint="An inactive provider can't be chosen for new links. Its links keep it." /></div>}
        <div className="sm:col-span-2"><FormError error={save.error} /></div>
      </form>
    </Modal>
  )
}
