import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { RoutingDomainSelect } from '../../components/SearchSelect'
import { Button, EmptyState, Field, FormError, Input, Modal, Mono, Select, useConfirmSave } from '../../components/ui'
import { useChecks } from '../../components/workflow'
import {  post, put, fieldIssues } from '../../lib/api'
import type { Connection, ConnectorKind } from '../../lib/types'
import { between, isHttpsUrl, issues as check, isUUID, required } from '../../lib/validation'
import { blank } from './shared'
import type { Form } from './shared'

export function ConnectionForm({ open, editing, kinds, onClose }: { open: boolean; editing: Connection | null; kinds: ConnectorKind[]; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState<Form>(blank)
  useEffect(() => {
    if (open) setForm(editing ? { ...blank, ...editing, credentials: {} } : blank)
  }, [open, editing])
  const kind = kinds.find((k) => k.kind === form.kind)
  const confirmSave = useConfirmSave()
  const save = useMutation({ meta: { success: 'Connection saved.', inlineError: true },
    mutationFn: () => {
      // Leaving credentials empty on an edit keeps the stored ones.
      const credentials = Object.values(form.credentials).some(Boolean) ? form.credentials : undefined
      const body = { ...form, credentials, routingDomains: editing?.routingDomains ?? {} }
      return editing ? put(`/integrations/connections/${editing.id}`, body) : post('/integrations/connections', body)
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['connections'] }); onClose() },
  })
  const needed = (options: ConnectorKind['config'], values: Record<string, string>) =>
    Object.fromEntries((options ?? []).filter((o) => o.required).map((o) => [o.key, required(values[o.key], o.label)]))
  const guid = (key: string) => (form.config[key]?.trim() && !isUUID(form.config[key]) ? 'Use the ID in its GUID form.' : undefined)
  const azure = form.kind === 'azure'
  const aws = form.kind === 'aws'
  const accessKey = form.credentials.accessKeyId?.trim() ?? ''
  const secretKey = form.credentials.secretAccessKey?.trim() ?? ''
  const checks = useChecks(check({
    name: required(form.name, 'Name'),
    baseUrl: kind?.needsUrl ? (isHttpsUrl(form.baseUrl.trim()) ? undefined : 'Use a full https URL, such as https://fw1.corp.example.') : undefined,
    syncIntervalMinutes: between(form.syncIntervalMinutes, 5, 10080, 'Sync interval'),
    ...needed(kind?.config ?? [], form.config),
    // On an edit, an empty credential keeps the stored one.
    ...(editing?.hasCredentials ? {} : needed(kind?.credentials ?? [], form.credentials)),
    ...(azure ? { tenantId: form.config.tenantId?.trim() ? guid('tenantId') : required('', 'Tenant ID'), clientId: form.config.clientId?.trim() ? guid('clientId') : required('', 'Client ID'),
      subscriptionId: form.config.subscriptionId?.trim() ? guid('subscriptionId') : required('', 'Subscription ID') } : {}),
    ...(aws ? {
      roleArn: /^arn:aws[\w-]*:iam::\d{12}:role\/[\w+=,.@/-]+$/.test(form.config.roleArn?.trim() ?? '') ? undefined : 'Use a role ARN such as arn:aws:iam::123456789012:role/df-ipam-read.',
      regions: (form.config.regions ?? '').split(',').map((r) => r.trim()).filter(Boolean).every((r) => /^[a-z]{2}(-[a-z]+)+-\d$/.test(r)) && form.config.regions?.trim() ? undefined : 'List regions such as ap-south-1, separated by commas.',
      secretAccessKey: accessKey && !secretKey ? 'Enter the secret key with its access key.' : undefined,
      accessKeyId: secretKey && !accessKey ? 'Enter the access key with its secret key.' : undefined,
    } : {}),
  }), open)
  const errors = { ...fieldIssues(save.error), ...checks.shown }
  const setMap = (group: 'config' | 'credentials', key: string, value: string) => setForm((f) => ({ ...f, [group]: { ...f[group], [key]: value } }))
  return (
    <Modal open={open} onClose={onClose} title={editing ? `Edit ${editing.name}` : 'Add a connection'} size="lg"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} disabled={!kind?.available} onClick={() => checks.submit(() => editing ? confirmSave(`connection ${editing.name}`, () => save.mutate()) : save.mutate())}>Save</Button></>}>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Connector" htmlFor="conn-kind">
          <Select id="conn-kind" value={form.kind} disabled={editing !== null} onChange={(e) => setForm({ ...blank, name: form.name, kind: e.target.value })}>
            {kinds.map((k) => <option key={k.kind} value={k.kind} disabled={!k.available}>{k.label}{k.available ? '' : ' (waiting)'}</option>)}
          </Select>
        </Field>
        <Field label="Name" htmlFor="conn-name" error={errors.name}><Input id="conn-name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></Field>
      </div>
      {kind && <p className="mt-2 text-[13px] text-ink-2">{kind.note}</p>}
      {kind?.needsUrl && (
        <div className="mt-3"><Field label="URL" htmlFor="conn-url" error={errors.baseUrl} hint="https only, on the server's approved host list."><Input id="conn-url" value={form.baseUrl} onChange={(e) => setForm({ ...form, baseUrl: e.target.value })} /></Field></div>
      )}
      <div className="mt-3 grid gap-3 sm:grid-cols-2">
        {kind?.config?.map((o) => (
          <Field key={o.key} label={o.required ? o.label : `${o.label} (optional)`} htmlFor={`conn-${o.key}`} error={errors[o.key]} hint={o.hint}>
            <Input id={`conn-${o.key}`} value={form.config[o.key] ?? ''} onChange={(e) => setMap('config', o.key, e.target.value)} />
          </Field>
        ))}
        {kind?.credentials?.map((o) => (
          <Field key={o.key} label={o.required ? o.label : `${o.label} (optional)`} htmlFor={`cred-${o.key}`} error={errors[o.key]} hint={editing?.hasCredentials ? 'Stored. Leave empty to keep it.' : undefined}>
            <Input id={`cred-${o.key}`} type="password" autoComplete="off" value={form.credentials[o.key] ?? ''} onChange={(e) => setMap('credentials', o.key, e.target.value)} />
          </Field>
        ))}
        <Field label="Sync every (minutes)" htmlFor="conn-interval" error={errors.syncIntervalMinutes}>
          <Input id="conn-interval" type="number" min={5} max={10080} value={form.syncIntervalMinutes} onChange={(e) => setForm({ ...form, syncIntervalMinutes: Number(e.target.value) })} />
        </Field>
      </div>
      <label className="mt-3 flex items-center gap-2 text-ink"><input type="checkbox" className="size-4 accent-[var(--accent)]" checked={form.isEnabled} onChange={(e) => setForm({ ...form, isEnabled: e.target.checked })} /> Sync on schedule</label>
      {Object.keys(errors).length === 0 && <div className="mt-3"><FormError error={save.error} /></div>}
      <p className="mt-3 text-[12px] text-ink-3">Credentials are encrypted and can't be read back. Use an account that can only read.</p>
    </Modal>
  )
}

export function RoutingMap({ connection, onClose }: { connection: Connection | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [map, setMap] = useState<Record<string, string>>({})
  useEffect(() => { setMap(connection?.routingDomains ?? {}) }, [connection])
  const confirmSave = useConfirmSave()
  const save = useMutation({ meta: { success: 'Routing map saved.', inlineError: true },
    mutationFn: () => put(`/integrations/connections/${connection?.id}`, { ...connection, routingDomains: map }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['connections'] }); onClose() },
  })
  if (!connection) return null
  const contexts = [...new Set([...connection.contexts, ...Object.keys(connection.routingDomains)])].sort()
  return (
    <Modal open onClose={onClose} title="Routing domains" subtitle={connection.name} size="lg"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} disabled={contexts.length === 0} onClick={() => confirmSave(`the routing map of ${connection?.name}`, () => save.mutate())}>Save</Button></>}>
      <p className="mb-3 text-[13px] text-ink-2">Evidence from a place with no routing domain is kept, but isn't matched to inventory. Nothing is mapped automatically, because the same address can exist in two isolated networks.</p>
      {contexts.length === 0 && <EmptyState title="Nothing to map yet" body="Run a sync first. The VDOMs and networks it finds appear here." />}
      <div className="space-y-2">
        {contexts.map((c) => (
          <div key={c} className="grid items-center gap-2 sm:grid-cols-2">
            <Mono>{c}</Mono>
            <RoutingDomainSelect aria-label={`Routing domain for ${c}`} value={map[c] ?? ''} onChange={(v) => setMap({ ...map, [c]: v })} emptyLabel="Not mapped" />
          </div>
        ))}
      </div>
      <div className="mt-3"><FormError error={save.error} /></div>
    </Modal>
  )
}
