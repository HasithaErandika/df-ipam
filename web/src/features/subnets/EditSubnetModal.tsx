import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { SearchSelect } from '../../components/SearchSelect'
import { Button, Field, FormError, Input, Modal, Mono, Select, useConfirmSave } from '../../components/ui'
import { useChecks } from '../../components/workflow'
import { listAll, patch, fieldIssues } from '../../lib/api'
import { environments } from '../../lib/format'
import type { Subnet } from '../../lib/types'
import { between, issues as check, optionalIP, required } from '../../lib/validation'

interface Option { id: string; name: string; email?: string; vlanId?: number; cidr?: string }

const useOptions = (path: string) => useQuery({ queryKey: ['options', path], queryFn: () => listAll<Option>(path) })

const mappingFields: { key: string; label: string; path: string; ref: keyof Subnet; label2?: (o: Option) => string }[] = [
  { key: 'networkBlockId', label: 'Network block', path: '/network-blocks', ref: 'networkBlock', label2: (o) => `${o.cidr} ${o.name}` },
  { key: 'siteId', label: 'Site', path: '/sites', ref: 'site' },
  { key: 'networkZoneId', label: 'Network zone', path: '/network-zones', ref: 'networkZone' },
  { key: 'vdomId', label: 'VDOM', path: '/vdoms', ref: 'vdom' },
  { key: 'vlanId', label: 'VLAN', path: '/vlans', ref: 'vlan', label2: (o) => `${o.vlanId} ${o.name}` },
  { key: 'businessUnitId', label: 'Business unit', path: '/business-units', ref: 'businessUnit' },
  { key: 'businessServiceId', label: 'Business service', path: '/business-services', ref: 'businessService' },
  { key: 'technicalOwnerId', label: 'Technical owner', path: '/people', ref: 'technicalOwner', label2: (o) => o.email ?? o.name },
  { key: 'businessOwnerId', label: 'Business owner', path: '/people', ref: 'businessOwner', label2: (o) => o.email ?? o.name },
]

function MappingSelect({ def, value, onChange, error }: { def: typeof mappingFields[number]; value: string; onChange: (v: string) => void; error?: string }) {
  const options = useOptions(def.path)
  const id = `edit-${def.key}`
  return (
    <Field label={def.label} htmlFor={id} error={error}>
      <SearchSelect id={id} value={value} onChange={onChange} emptyLabel="None" loading={options.isPending} fetchError={options.error} onRetry={() => options.refetch()}
        options={options.data?.map((o) => ({ value: o.id, label: def.label2 ? def.label2(o) : o.name })) ?? []} />
    </Field>
  )
}

/** Edits a subnet. CIDR and routing domain stay fixed, because IP records depend on them. */
export function EditSubnetModal({ subnet, onClose, onSaved }: { subnet: Subnet; onClose: () => void; onSaved: (s: Subnet) => void }) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState<Record<string, string | boolean>>(() => ({
    name: subnet.name, environment: subnet.environment, gateway: subnet.gateway ?? '', securityZone: subnet.securityZone,
    description: subnet.description, sensitivity: subnet.sensitivity, discoveryEnabled: subnet.discoveryEnabled,
    warnThreshold: String(subnet.warnThreshold), criticalThreshold: String(subnet.criticalThreshold),
    ...Object.fromEntries(mappingFields.map((m) => [m.key, (subnet[m.ref] as { id: string } | null)?.id ?? ''])),
  }))
  const confirmSave = useConfirmSave()
  const save = useMutation({ meta: { success: 'Subnet saved.', inlineError: true },
    mutationFn: () => patch<Subnet>(`/subnets/${subnet.id}`, {
      ...form, warnThreshold: Number(form.warnThreshold), criticalThreshold: Number(form.criticalThreshold),
    }, subnet.version),
    onSuccess: (s) => { queryClient.invalidateQueries({ queryKey: ['subnets'] }); onSaved(s); onClose() },
  })
  const warn = Number(form.warnThreshold)
  const checks = useChecks(check({
    name: required(String(form.name), 'Name'),
    gateway: optionalIP(String(form.gateway)),
    warnThreshold: between(String(form.warnThreshold), 1, 100, 'Warning level'),
    criticalThreshold: between(String(form.criticalThreshold), 1, 100, 'Critical level') ?? (Number(form.criticalThreshold) <= warn ? 'Critical must be above the warning level.' : undefined),
  }))
  const errors = { ...fieldIssues(save.error), ...checks.shown }
  const set = (k: string) => (v: string | boolean) => setForm((f) => ({ ...f, [k]: v }))
  const text = (k: string, label: string, mono = false) => (
    <Field label={label} htmlFor={`edit-${k}`} error={errors[k]}>
      <Input id={`edit-${k}`} className={mono ? 'font-mono' : undefined} value={String(form[k])} onChange={(e) => set(k)(e.target.value)} />
    </Field>
  )
  return (
    <Modal open onClose={onClose} size="xl" title={`Edit ${subnet.name}`} subtitle={<><Mono>{subnet.calculated.cidr}</Mono> · {subnet.routingDomainName}. The CIDR and routing domain stay fixed.</>}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => checks.submit(() => confirmSave(subnet.name, () => save.mutate()))}>Save changes</Button></>}>
      <div className="grid grid-cols-2 gap-3 md:grid-cols-3">
        {text('name', 'Name')}
        <Field label="Environment" htmlFor="edit-environment" error={errors.environment}>
          <Select id="edit-environment" value={String(form.environment)} onChange={(e) => set('environment')(e.target.value)}>
            {Object.entries(environments).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </Select>
        </Field>
        {text('gateway', 'Gateway', true)}
        {text('securityZone', 'Security zone')}
        <Field label="Sensitivity" htmlFor="edit-sensitivity" hint="Sensitive and restricted subnets stay passive for discovery.">
          <Select id="edit-sensitivity" value={String(form.sensitivity)} onChange={(e) => set('sensitivity')(e.target.value)}>
            <option value="standard">Standard</option><option value="sensitive">Sensitive</option><option value="restricted">Restricted</option>
          </Select>
        </Field>
        {text('description', 'Description')}
        {text('warnThreshold', 'Warning at (%)')}
        {text('criticalThreshold', 'Critical at (%)')}
        <label className="flex items-center gap-2 self-end pb-2 text-[13px] text-ink">
          <input type="checkbox" checked={Boolean(form.discoveryEnabled)} onChange={(e) => set('discoveryEnabled')(e.target.checked)} /> Discovery enabled
        </label>
        {mappingFields.map((m) => <MappingSelect key={m.key} def={m} value={String(form[m.key])} onChange={set(m.key)} error={errors[m.key]} />)}
      </div>
      {Object.keys(errors).length === 0 && <div className="mt-3"><FormError error={save.error} /></div>}
    </Modal>
  )
}
