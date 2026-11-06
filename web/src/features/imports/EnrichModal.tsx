import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { Button, Field, FormError, Input, Modal, Mono, useConfirmSave } from '../../components/ui'
import { useChecks } from '../../components/workflow'
import { patchPlain } from '../../lib/api'
import { between, cidr as cidrIssue, issues as check, optionalIP } from '../../lib/validation'
import { useRefresh } from './shared'
import type { ImportRecord } from './shared'

export function EnrichModal({ record, onClose }: { record: ImportRecord; onClose: () => void }) {
  const refresh = useRefresh()
  const [form, setForm] = useState({ cidr: record.parsed.cidr ?? '', name: record.parsed.name ?? '', gateway: record.parsed.gateway ?? '', vdom: record.parsed.vdom ?? '', vlan: record.parsed.vlan ?? '' })
  const confirmSave = useConfirmSave()
  const save = useMutation({ meta: { success: 'Line corrected.', inlineError: true }, mutationFn: () => patchPlain<ImportRecord>(`/imports/records/${record.id}`, form), onSuccess: () => { refresh(); onClose() } })
  const set = (k: keyof typeof form) => (e: { target: { value: string } }) => setForm({ ...form, [k]: e.target.value })
  const checks = useChecks(check({ cidr: form.cidr.trim() ? cidrIssue(form.cidr) : undefined, gateway: optionalIP(form.gateway), vlan: form.vlan.trim() ? between(form.vlan, 1, 4094, 'VLAN') : undefined }))
  return (
    <Modal open onClose={onClose} title={`Correct line ${record.lineNumber}`} subtitle={<Mono>{record.rawText}</Mono>}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => checks.submit(() => confirmSave(`line ${record.lineNumber}`, () => save.mutate()))}>Save and recheck</Button></>}>
      <div className="grid grid-cols-2 gap-3">
        <Field label="CIDR" htmlFor="en-cidr" error={checks.shown.cidr}><Input id="en-cidr" className="font-mono" value={form.cidr} onChange={set('cidr')} /></Field>
        <Field label="Name" htmlFor="en-name"><Input id="en-name" value={form.name} onChange={set('name')} /></Field>
        <Field label="Gateway" htmlFor="en-gw" error={checks.shown.gateway}><Input id="en-gw" className="font-mono" value={form.gateway} onChange={set('gateway')} /></Field>
        <Field label="VDOM" htmlFor="en-vdom"><Input id="en-vdom" value={form.vdom} onChange={set('vdom')} /></Field>
        <Field label="VLAN" htmlFor="en-vlan" error={checks.shown.vlan}><Input id="en-vlan" value={form.vlan} onChange={set('vlan')} /></Field>
      </div>
      <p className="mt-2 text-[12.5px] text-ink-3">The original line is kept unchanged for the record.</p>
      <div className="mt-2"><FormError error={save.error} /></div>
    </Modal>
  )
}
