import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Badge, Button, Field, FormError, Input, Modal, Mono, Select, SkeletonRows, Td, Th } from '../components/ui'
import { Tabs, useChecks } from '../components/workflow'
import { ApiError, get, listAll, patch, post } from '../lib/api'
import { environments } from '../lib/format'
import type { Subnet } from '../lib/types'
import { RoutingDomainSelect, SearchSelect } from '../components/SearchSelect'
import { between, choose, cidr as cidrIssue, ip as ipIssue, issues as check, optionalIP, required } from '../lib/validation'

interface Option { id: string; name: string; email?: string; vlanId?: number; cidr?: string }

const useOptions = (path: string) => useQuery({ queryKey: ['options', path], queryFn: () => listAll<Option>(path) })

function issues(error: unknown): Record<string, string> {
  const out: Record<string, string> = {}
  if (error instanceof ApiError) for (const d of error.details) out[d.field] = d.issue
  return out
}

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
  const save = useMutation({
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
  const errors = { ...issues(save.error), ...checks.shown }
  const set = (k: string) => (v: string | boolean) => setForm((f) => ({ ...f, [k]: v }))
  const text = (k: string, label: string, mono = false) => (
    <Field label={label} htmlFor={`edit-${k}`} error={errors[k]}>
      <Input id={`edit-${k}`} className={mono ? 'font-mono' : undefined} value={String(form[k])} onChange={(e) => set(k)(e.target.value)} />
    </Field>
  )
  return (
    <Modal open onClose={onClose} size="xl" title={`Edit ${subnet.name}`} subtitle={<><Mono>{subnet.calculated.cidr}</Mono> · {subnet.routingDomainName}. The CIDR and routing domain stay fixed.</>}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => checks.submit(() => save.mutate())}>Save changes</Button></>}>
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

interface BulkResult { row: number; cidr: string; name: string; ok: boolean; code?: string; issue?: string }
interface BulkResponse { created: boolean; valid: number; invalid: number; results: BulkResult[] }

/** Parses "cidr, name, gateway" lines. Blank lines and # comments are skipped. */
export function parseBulk(text: string) {
  return text.split('\n').map((l) => l.trim()).filter((l) => l && !l.startsWith('#')).map((line) => {
    const [cidr = '', name = '', gateway = ''] = line.split(/[,\t]/).map((p) => p.trim())
    return { cidr, name: name || cidr, gateway: gateway || null }
  })
}

export function BulkCreateModal({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient()
  const [routingDomainId, setRd] = useState('')
  const [environment, setEnvironment] = useState('')
  const [text, setText] = useState('')
  const [result, setResult] = useState<BulkResponse | null>(null)
  const run = useMutation({
    mutationFn: (dryRun: boolean) => post<BulkResponse>('/subnets/bulk', { routingDomainId, environment, dryRun, subnets: parseBulk(text) }),
    onSuccess: (r) => { setResult(r); if (r.created) queryClient.invalidateQueries({ queryKey: ['subnets'] }) },
  })
  const rows = parseBulk(text)
  const checks = useChecks(check({
    routingDomainId: choose(routingDomainId, 'a routing domain'), environment: choose(environment, 'an environment'),
    subnets: rows.length === 0 ? 'Add at least one subnet.' : undefined,
  }))
  const errors = { ...issues(run.error), ...checks.shown }
  return (
    <Modal open onClose={onClose} size="xl" title="Bulk create subnets" subtitle="Check first, then create. All rows are created together, or none are."
      footer={result?.created ? <Button variant="primary" onClick={onClose}>Done</Button> : <>
        <Button onClick={onClose}>Cancel</Button>
        <Button busy={run.isPending && run.variables === true} disabled={run.isPending} onClick={() => checks.submit(() => run.mutate(true))}>Check {rows.length} rows</Button>
        <Button variant="primary" busy={run.isPending && run.variables === false} disabled={run.isPending || !result || result.invalid > 0} onClick={() => run.mutate(false)}>Create {rows.length} subnets</Button>
      </>}>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Routing domain" htmlFor="bulk-rd" error={errors.routingDomainId}>
          <RoutingDomainSelect id="bulk-rd" value={routingDomainId} onChange={(v) => { setRd(v); setResult(null) }} />
        </Field>
        <Field label="Environment" htmlFor="bulk-env" error={errors.environment}>
          <Select id="bulk-env" value={environment} onChange={(e) => { setEnvironment(e.target.value); setResult(null) }}>
            <option value="">Select</option>
            {Object.entries(environments).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </Select>
        </Field>
        <div className="col-span-2">
          <Field label="Subnets, one per line" htmlFor="bulk-text" hint="CIDR, name, gateway (optional). Commas or tabs, so a spreadsheet column paste works." error={errors.subnets}>
            <textarea id="bulk-text" rows={7} spellCheck={false} value={text} onChange={(e) => { setText(e.target.value); setResult(null) }}
              placeholder={'10.40.1.0/24, App tier, 10.40.1.1\n10.40.2.0/24, DB tier'}
              className="w-full rounded-[5px] border border-line-strong bg-surface-2 px-2.5 py-1.5 font-mono text-[12.5px] text-ink focus:border-accent focus:bg-surface focus:outline-none" />
          </Field>
        </div>
      </div>
      {Object.keys(errors).length === 0 && <div className="mt-3"><FormError error={run.error} /></div>}
      {result && (
        <div className="mt-3">
          <p className={`mb-2 text-[13px] ${result.created ? 'text-ok' : result.invalid ? 'text-danger' : 'text-ink-2'}`}>
            {result.created ? `Created ${result.valid} subnets.` : result.invalid ? `${result.invalid} of ${result.results.length} rows need fixing. Nothing was saved.` : `All ${result.valid} rows are valid. Nothing is saved until you create them.`}
          </p>
          <div className="max-h-64 overflow-y-auto rounded-[5px] border border-line">
            <table className="w-full border-collapse">
              <thead><tr><Th>Row</Th><Th>CIDR</Th><Th>Name</Th><Th>Result</Th></tr></thead>
              <tbody>{result.results.map((r) => (
                <tr key={r.row}><Td className="tabular-nums">{r.row}</Td><Td><Mono>{r.cidr}</Mono></Td><Td>{r.name}</Td>
                  <Td>{r.ok ? <Badge tone="ok">{result.created ? 'Created' : 'Valid'}</Badge> : <span className="text-[12.5px] text-danger">{r.issue}</span>}</Td></tr>
              ))}</tbody>
            </table>
          </div>
        </div>
      )}
    </Modal>
  )
}

interface Located { subnetId: string; subnetName: string; cidr: string; routingDomainName: string }

/** Split, summarise and locate. Nothing here changes inventory. */
export function ToolsModal({ onClose, onOpenSubnet }: { onClose: () => void; onOpenSubnet: (id: string) => void }) {
  const [tab, setTab] = useState('split')
  const [cidr, setCidr] = useState('')
  const [prefix, setPrefix] = useState('')
  const [many, setMany] = useState('')
  const [address, setAddress] = useState('')
  const split = useMutation({ mutationFn: () => post<{ subnets: string[]; total: string; truncated: boolean }>('/tools/split', { cidr, newPrefix: Number(prefix) }) })
  const summarise = useMutation({ mutationFn: () => post<{ summary: string[] }>('/tools/summarise', { cidrs: many.split(/[\s,]+/).filter(Boolean) }) })
  const locate = useMutation({ mutationFn: () => get<Located[]>(`/tools/locate?address=${encodeURIComponent(address.trim())}`) })
  const splitChecks = useChecks(check({ cidr: cidrIssue(cidr), newPrefix: between(prefix, 0, 128, 'New prefix') }))
  const locateChecks = useChecks(check({ address: ipIssue(address) }))
  return (
    <Modal open onClose={onClose} size="lg" title="Subnet tools" subtitle="Calculations only. Nothing here changes inventory.">
      <Tabs tabs={[{ key: 'split', label: 'Split' }, { key: 'summarise', label: 'Summarise' }, { key: 'locate', label: 'Locate an address' }]} active={tab} onChange={setTab} />
      {tab === 'split' && (
        <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); splitChecks.submit(() => split.mutate()) }}>
          <div className="grid grid-cols-[1fr_8rem_auto] items-end gap-2">
            <Field label="CIDR" htmlFor="t-cidr" error={splitChecks.shown.cidr ?? issues(split.error).cidr}><Input id="t-cidr" className="font-mono" value={cidr} onChange={(e) => setCidr(e.target.value)} placeholder="10.0.0.0/22" /></Field>
            <Field label="New prefix" htmlFor="t-prefix" error={splitChecks.shown.newPrefix ?? issues(split.error).newPrefix}><Input id="t-prefix" type="number" value={prefix} onChange={(e) => setPrefix(e.target.value)} placeholder="24" /></Field>
            <Button type="submit" variant="primary" busy={split.isPending}>Split</Button>
          </div>
          {split.data && <><p className="text-[13px] text-ink-2">{split.data.total} subnets{split.data.truncated && `, showing the first ${split.data.subnets.length}`}.</p>
            <div className="flex max-h-56 flex-wrap gap-1 overflow-y-auto">{split.data.subnets.map((s) => <Badge key={s}><Mono>{s}</Mono></Badge>)}</div></>}
        </form>
      )}
      {tab === 'summarise' && (
        <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); summarise.mutate() }}>
          <Field label="CIDRs" htmlFor="t-many" hint="Separated by spaces, commas or new lines." error={issues(summarise.error).cidr ?? issues(summarise.error).cidrs}>
            <textarea id="t-many" rows={5} value={many} onChange={(e) => setMany(e.target.value)} spellCheck={false}
              className="w-full rounded-[5px] border border-line-strong bg-surface-2 px-2.5 py-1.5 font-mono text-[12.5px] focus:border-accent focus:outline-none" />
          </Field>
          <Button type="submit" variant="primary" busy={summarise.isPending}>Summarise</Button>
          {summarise.data && <div className="flex flex-wrap gap-1">{summarise.data.summary.map((s) => <Badge key={s} tone="accent"><Mono>{s}</Mono></Badge>)}</div>}
        </form>
      )}
      {tab === 'locate' && (
        <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); locateChecks.submit(() => locate.mutate()) }}>
          <div className="grid grid-cols-[1fr_auto] items-end gap-2">
            <Field label="Address" htmlFor="t-addr" error={locateChecks.shown.address ?? issues(locate.error).address}><Input id="t-addr" className="font-mono" value={address} onChange={(e) => setAddress(e.target.value)} /></Field>
            <Button type="submit" variant="primary" busy={locate.isPending}>Locate</Button>
          </div>
          {locate.isPending && <SkeletonRows rows={2} />}
          {locate.data && (locate.data.length === 0 ? <p className="text-[13px] text-ink-3">No active subnet holds this address in any routing domain.</p> : (
            <ul>{locate.data.map((m) => (
              <li key={m.subnetId} className="flex items-center justify-between border-b border-line py-1.5 text-[13px] last:border-0">
                <span><Mono>{m.cidr}</Mono> {m.subnetName} <span className="text-ink-3">· {m.routingDomainName}</span></span>
                <Button variant="ghost" className="h-7" onClick={() => onOpenSubnet(m.subnetId)}>Open</Button>
              </li>
            ))}</ul>
          ))}
          <p className="text-[12px] text-ink-3">The same private address can sit in several routing domains, so each one is listed.</p>
        </form>
      )}
    </Modal>
  )
}
