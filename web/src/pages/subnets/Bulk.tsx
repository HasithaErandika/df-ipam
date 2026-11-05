import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useState, type FormEvent } from 'react'
import { Button, ConfirmDialog, Field, FormError, Modal, Select } from '../../components/ui'
import { post } from '../../lib/api'
import { environments } from '../../lib/format'
import { optionLabel, useOptions } from '../../lib/hooks'
import { sensitivities } from './SubnetForm'

const keep = '__keep'
const clear = '__clear'

function invalidate(qc: ReturnType<typeof useQueryClient>) {
  qc.invalidateQueries({ queryKey: ['subnets'] })
  qc.invalidateQueries({ queryKey: ['summary'] })
}

/** One set of changes for every selected subnet, applied all or nothing. */
export function BulkEdit({ ids, onClose, onDone }: { ids: string[]; onClose: () => void; onDone: () => void }) {
  const qc = useQueryClient()
  const opt = {
    siteId: useOptions('/sites'), networkZoneId: useOptions('/network-zones'), vlanId: useOptions('/vlans'), vdomId: useOptions('/vdoms'),
    businessUnitId: useOptions('/business-units'), businessServiceId: useOptions('/business-services'),
    technicalOwnerId: useOptions('/users/lookup'), businessOwnerId: useOptions('/users/lookup'),
  }
  const labels: Record<keyof typeof opt, string> = {
    siteId: 'Site', networkZoneId: 'Network zone', vlanId: 'VLAN', vdomId: 'VDOM', businessUnitId: 'Business unit',
    businessServiceId: 'Business service', technicalOwnerId: 'Technical owner', businessOwnerId: 'Business owner',
  }
  const [values, setValues] = useState<Record<string, string>>({})
  const value = (k: string) => values[k] ?? keep
  const set = (k: string) => (e: { target: { value: string } }) => setValues((p) => ({ ...p, [k]: e.target.value }))
  const changes = () => {
    const out: Record<string, unknown> = {}
    for (const [k, v] of Object.entries(values)) {
      if (v === keep) continue
      if (k === 'discoveryEnabled') out[k] = v === 'true'
      else out[k] = v === clear ? '' : v
    }
    return out
  }
  const save = useMutation({
    mutationFn: () => post<{ affected: number }>('/subnets/bulk', { ids, action: 'update', changes: changes() }),
    onSuccess: () => { invalidate(qc); onDone(); onClose() },
  })
  const submit = (e: FormEvent) => { e.preventDefault(); save.mutate() }
  const none = Object.keys(changes()).length === 0
  return (
    <Modal open onClose={onClose} size="lg" title={`Edit ${ids.length} subnet${ids.length === 1 ? '' : 's'}`}
      subtitle="Only the fields you change are applied. If any subnet can't take the change, none are changed."
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" type="submit" form="bulk-edit" disabled={none} busy={save.isPending}>Apply to {ids.length}</Button></>}>
      <form id="bulk-edit" onSubmit={submit} className="grid gap-3 sm:grid-cols-2">
        <Field label="Environment" htmlFor="bulk-environment">
          <Select id="bulk-environment" value={value('environment')} onChange={set('environment')}>
            <option value={keep}>Keep as is</option>
            {Object.entries(environments).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </Select>
        </Field>
        <Field label="Sensitivity" htmlFor="bulk-sensitivity">
          <Select id="bulk-sensitivity" value={value('sensitivity')} onChange={set('sensitivity')}>
            <option value={keep}>Keep as is</option>
            {Object.entries(sensitivities).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </Select>
        </Field>
        <Field label="Discovery" htmlFor="bulk-discovery">
          <Select id="bulk-discovery" value={value('discoveryEnabled')} onChange={set('discoveryEnabled')}>
            <option value={keep}>Keep as is</option><option value="true">Include</option><option value="false">Don't include</option>
          </Select>
        </Field>
        {(Object.keys(opt) as (keyof typeof opt)[]).map((k) => (
          <Field key={k} label={labels[k]} htmlFor={`bulk-${k}`}>
            <Select id={`bulk-${k}`} value={value(k)} onChange={set(k)}>
              <option value={keep}>Keep as is</option>
              <option value={clear}>Clear</option>
              {opt[k].data?.map((o) => <option key={o.id} value={o.id}>{optionLabel(o)}</option>)}
            </Select>
          </Field>
        ))}
        <div className="sm:col-span-2"><FormError error={save.error} /></div>
      </form>
    </Modal>
  )
}

/** Archive or delete every selected subnet, all or nothing. */
export function BulkConfirm({ ids, action, onClose, onDone }: { ids: string[]; action: 'archive' | 'delete'; onClose: () => void; onDone: () => void }) {
  const qc = useQueryClient()
  const run = useMutation({
    mutationFn: () => post('/subnets/bulk', { ids, action }),
    onSuccess: () => { invalidate(qc); onDone(); onClose() },
  })
  const n = `${ids.length} subnet${ids.length === 1 ? '' : 's'}`
  return (
    <ConfirmDialog open title={action === 'delete' ? `Delete ${n}?` : `Archive ${n}?`}
      body={action === 'delete'
        ? "They are removed for good. If any has IP records or other linked data, nothing is deleted and you're told which one."
        : 'They leave the active inventory and their ranges can be reused. History is kept.'}
      confirmLabel={action === 'delete' ? 'Delete' : 'Archive'} busy={run.isPending} error={run.error}
      onConfirm={() => run.mutate()} onCancel={onClose} />
  )
}
