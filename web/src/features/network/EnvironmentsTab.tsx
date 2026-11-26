import { useMutation } from '@tanstack/react-query'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Badge, Button, Card, DataTable, EnvironmentBadge, Field, FormError, Input, Modal, RowMenu, Select, SeverityBadge, Switch, useConfirm, useConfirmSave, type Column } from '../../components/ui'
import { del, post, put } from '../../lib/api'
import { listChanges } from '../../lib/changes'
import { label, options, severities } from '../../lib/labels'
import { useEnvironments, useInvalidate } from '../../lib/queries'
import type { Environment } from '../../lib/types'

// A stored colour (the picker needs hex): the accent, --accent in index.css.
const blank = { name: '', colour: '#2257c9', sortOrder: 100, defaultSeverity: 'medium', defaultMonitor: false, defaultCritical: false }
type Values = typeof blank

function EnvironmentForm({ open, env, onClose }: { open: boolean; env?: Environment; onClose: () => void }) {
  const [v, setV] = useState<Values>(blank)
  const refresh = useInvalidate('/environments', '/ip-addresses', '/dashboard')
  const confirmSave = useConfirmSave()
  useEffect(() => { if (open) setV(env ? { ...env } : blank) }, [open, env])
  const save = useMutation({
    mutationFn: () => (env ? put(`/environments/${env.id}`, v) : post('/environments', v)),
    meta: { inlineError: true, success: `${v.name} saved.` }, onSuccess: () => { void refresh(); onClose() },
  })
  const set = <K extends keyof Values>(k: K, value: Values[K]) => setV((x) => ({ ...x, [k]: value }))
  const changes = env ? listChanges({ ...blank, ...env }, v, { name: 'Name', colour: 'Colour', sortOrder: 'Order', defaultSeverity: 'Alert severity',
    defaultMonitor: 'Monitor new servers', defaultCritical: 'Mark new servers critical' }, { defaultSeverity: (x) => label(severities, String(x)) }) : []
  const nameIssue = !v.name.trim() ? 'Name is required.' : v.name.length > 40 ? 'Keep the name under 40 characters.' : undefined
  return (
    <Modal open={open} onClose={onClose} title={env ? `Edit ${env.name}` : 'Add an environment'} subtitle="Defaults apply to new servers. Each server can still be set by hand."
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" disabled={Boolean(nameIssue) || (Boolean(env) && changes.length === 0)} busy={save.isPending}
        onClick={() => (env ? void confirmSave(env.name, () => save.mutate(), undefined, changes) : save.mutate())}>{env ? 'Save changes' : 'Add environment'}</Button></>}>
      <div className="space-y-4">
        <div className="grid gap-3 sm:grid-cols-[1fr_auto_auto]">
          <Field label="Name" htmlFor="ev-name" error={v.name ? nameIssue : undefined}><Input id="ev-name" value={v.name} placeholder="Staging" onChange={(e) => set('name', e.target.value)} /></Field>
          <Field label="Colour" htmlFor="ev-col"><input id="ev-col" type="color" value={v.colour} onChange={(e) => set('colour', e.target.value)} className="h-8 w-14 cursor-pointer rounded border border-line-strong" /></Field>
          <Field label="Order" htmlFor="ev-ord"><Input id="ev-ord" type="number" min={0} max={10000} className="w-20" value={v.sortOrder} onChange={(e) => set('sortOrder', Number(e.target.value))} /></Field>
        </div>
        <Field label="Alert severity when down or MAC changes" htmlFor="ev-sev">
          <Select id="ev-sev" value={v.defaultSeverity} onChange={(e) => set('defaultSeverity', e.target.value)}>{options(severities).map((o) => <option key={o.value} value={o.value}>{o.text}</option>)}</Select>
        </Field>
        <Switch id="ev-mon" checked={v.defaultMonitor} onChange={(x) => setV((s) => ({ ...s, defaultMonitor: x, defaultCritical: x && s.defaultCritical }))} label="Monitor new servers" />
        <Switch id="ev-crit" checked={v.defaultCritical} disabled={!v.defaultMonitor} onChange={(x) => set('defaultCritical', x)} label="Mark new servers critical" hint="Critical servers raise Critical alerts whatever the severity above." />
        <FormError error={save.error} />
      </div>
    </Modal>
  )
}

export function EnvironmentsTab() {
  const q = useEnvironments()
  const [form, setForm] = useState<{ env?: Environment } | null>(null)
  const confirm = useConfirm()
  const refresh = useInvalidate('/environments')
  const remove = useMutation({ mutationFn: (e: Environment) => del(`/environments/${e.id}`), meta: { success: (e: Environment) => `${e.name} deleted.` }, onSuccess: () => void refresh() })
  const columns: Column<Environment>[] = [
    { key: 'name', header: 'Environment', cell: (e) => <EnvironmentBadge name={e.name} colour={e.colour} /> },
    { key: 'sev', header: 'Down or MAC change', cell: (e) => <SeverityBadge severity={e.defaultSeverity} /> },
    { key: 'defaults', header: 'New servers', cell: (e) => <div className="flex gap-1">{e.defaultMonitor ? <Badge tone="accent">Monitored</Badge> : <Badge>Not monitored</Badge>}{e.defaultCritical && <Badge tone="danger">Critical</Badge>}</div> },
    { key: 'use', header: 'In use', className: 'text-right tabular-nums', cell: (e) => e.inUse },
    { key: 'act', header: '', className: 'w-10 text-right', cell: (e) => (
      <RowMenu actions={[
        { label: 'Edit', icon: <Pencil size={14} aria-hidden />, onSelect: () => setForm({ env: e }) },
        { label: 'Delete', icon: <Trash2 size={14} aria-hidden />, danger: true, hidden: e.inUse > 0, onSelect: async () => {
          if (await confirm({ title: `Delete ${e.name}?`, body: 'Segments that default to it lose the default.', confirmLabel: 'Delete environment' })) remove.mutate(e)
        } },
      ]} />
    ) },
  ]
  return (
    <Card title="Environments" subtitle="Production, UAT, DR and others: each sets the default alert severity for its servers."
      action={<Button variant="primary" onClick={() => setForm({})}><Plus size={14} aria-hidden />Add environment</Button>}>
      <DataTable columns={columns} rows={q.data ?? []} query={q} rowKey={(e) => e.id} empty={{ title: 'No environments' }} />
      <EnvironmentForm open={form !== null} env={form?.env} onClose={() => setForm(null)} />
    </Card>
  )
}
