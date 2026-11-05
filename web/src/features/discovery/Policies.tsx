import { useMutation, useQuery } from '@tanstack/react-query'
import { Plus, ShieldCheck, X } from 'lucide-react'
import { useState } from 'react'
import { Badge, Button, Card, EmptyState, ErrorState, Field, FormError, Input, Modal, Mono, MoreOptions, SkeletonRows, Td, Th, useConfirmSave } from '../../components/ui'
import { useChecks } from '../../components/workflow'
import {  list, post, put, fieldIssues } from '../../lib/api'
import { useCan } from '../../lib/session'
import { between, isCIDR, issues as check, required } from '../../lib/validation'
import { methodLabels, useRefresh } from './shared'
import type { Policy, Window } from './shared'

const dayNames = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']

function PolicyModal({ policy, onClose }: { policy: Policy | null; onClose: () => void }) {
  const refresh = useRefresh()
  const [form, setForm] = useState(() => ({
    name: policy?.name ?? '', description: policy?.description ?? '', methods: policy?.methods ?? ['icmp', 'arp', 'dns'],
    allowlist: policy?.targetAllowlist.join('\n') ?? '', maxPps: String(policy?.maxPps ?? 50), maxConcurrency: String(policy?.maxConcurrency ?? 16),
    timeoutMs: String(policy?.timeoutMs ?? 1000), retries: String(policy?.retries ?? 1), isSensitive: policy?.isSensitive ?? false,
    windows: policy?.windows ?? [] as Window[],
  }))
  const confirmSave = useConfirmSave()
  const save = useMutation({ meta: { success: 'Policy saved. It needs approval before it can run.', inlineError: true },
    mutationFn: () => {
      const body = { name: form.name, description: form.description, methods: form.methods, targetAllowlist: form.allowlist.split(/[\s,]+/).filter(Boolean),
        maxPps: Number(form.maxPps), maxConcurrency: Number(form.maxConcurrency), timeoutMs: Number(form.timeoutMs), retries: Number(form.retries),
        isSensitive: form.isSensitive, windows: form.windows }
      return policy ? put(`/discovery/policies/${policy.id}`, body) : post('/discovery/policies', body)
    },
    onSuccess: () => { refresh(); onClose() },
  })
  const networks = form.allowlist.split(/[\s,]+/).filter(Boolean)
  const checks = useChecks(check({
    name: required(form.name, 'Name'),
    methods: form.methods.length === 0 ? 'Choose at least one method.' : undefined,
    targetAllowlist: networks.length === 0 ? 'List at least one network.' : networks.find((n) => !isCIDR(n)) ? `${networks.find((n) => !isCIDR(n))} is not a valid network.` : undefined,
    maxPps: between(form.maxPps, 1, 500, 'Packets per second'),
    maxConcurrency: between(form.maxConcurrency, 1, 64, 'Probes at once'),
    timeoutMs: between(form.timeoutMs, 100, 30000, 'Timeout'),
    retries: between(form.retries, 0, 5, 'Retries'),
    windows: form.windows.some((w) => w.days.length === 0 || !w.start || !w.end) ? 'Each window needs at least one day, a start and an end.' : undefined,
  }))
  const errors = { ...fieldIssues(save.error), ...checks.shown }
  const setWindow = (i: number, w: Partial<Window>) => setForm((f) => ({ ...f, windows: f.windows.map((x, j) => (j === i ? { ...x, ...w } : x)) }))
  return (
    <Modal open onClose={onClose} size="lg" title={policy ? 'Edit policy' : 'New discovery policy'} subtitle="Saving needs a fresh approval from someone else before the policy runs again."
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => checks.submit(() => policy ? confirmSave(`policy ${policy.name}`, () => save.mutate(), "Saving clears its approval, so it can't run until someone approves it again.") : save.mutate())}>Save policy</Button></>}>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Name" htmlFor="pol-name" error={errors.name}><Input id="pol-name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></Field>
        <Field label="Description (optional)" htmlFor="pol-desc"><Input id="pol-desc" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} /></Field>
        <fieldset className="col-span-2">
          <legend className="mb-1 text-[13px] font-medium text-ink">Methods</legend>
          <div className="grid grid-cols-2 gap-1">
            {Object.entries(methodLabels).map(([k, v]) => (
              <label key={k} className="flex items-center gap-2 text-[13px] text-ink-2"><input type="checkbox" checked={form.methods.includes(k)}
                onChange={(e) => setForm({ ...form, methods: e.target.checked ? [...form.methods, k] : form.methods.filter((m) => m !== k) })} /> {v}</label>
            ))}
          </div>
          {errors.methods && <p className="text-[13px] text-danger">{errors.methods}</p>}
        </fieldset>
        <div className="col-span-2">
          <Field label="Approved networks" htmlFor="pol-allow" error={errors.targetAllowlist} hint="The only networks this policy may ever touch, one CIDR per line.">
            <textarea id="pol-allow" rows={3} spellCheck={false} value={form.allowlist} onChange={(e) => setForm({ ...form, allowlist: e.target.value })}
              className="w-full rounded-[5px] border border-line-strong bg-surface-2 px-2.5 py-1.5 font-mono text-[12.5px] focus:border-accent focus:outline-none" />
          </Field>
        </div>
        <div className="col-span-2">
          <MoreOptions label="Speed limits and time windows" forceOpen={Boolean(errors.maxPps || errors.maxConcurrency || errors.timeoutMs || errors.retries || errors.windows)}>
            <div className="grid grid-cols-2 gap-3">
        <Field label="Packets per second" htmlFor="pol-pps" error={errors.maxPps}><Input id="pol-pps" type="number" value={form.maxPps} onChange={(e) => setForm({ ...form, maxPps: e.target.value })} /></Field>
        <Field label="Probes at once" htmlFor="pol-conc" error={errors.maxConcurrency}><Input id="pol-conc" type="number" value={form.maxConcurrency} onChange={(e) => setForm({ ...form, maxConcurrency: e.target.value })} /></Field>
        <Field label="Timeout (ms)" htmlFor="pol-timeout" error={errors.timeoutMs}><Input id="pol-timeout" type="number" value={form.timeoutMs} onChange={(e) => setForm({ ...form, timeoutMs: e.target.value })} /></Field>
        <Field label="Retries" htmlFor="pol-retries" error={errors.retries}><Input id="pol-retries" type="number" value={form.retries} onChange={(e) => setForm({ ...form, retries: e.target.value })} /></Field>
        <fieldset className="col-span-2">
          <legend className="mb-1 text-[13px] font-medium text-ink">Time windows <span className="font-normal text-ink-3">(Colombo time; none means any time)</span></legend>
          {form.windows.map((w, i) => (
            <div key={i} className="mb-1.5 flex flex-wrap items-center gap-2">
              {dayNames.map((d, di) => (
                <label key={d} className="flex items-center gap-1 text-[12.5px] text-ink-2"><input type="checkbox" checked={w.days.includes(di + 1)}
                  onChange={(e) => setWindow(i, { days: e.target.checked ? [...w.days, di + 1] : w.days.filter((x) => x !== di + 1) })} />{d}</label>
              ))}
              <Input aria-label="Start" type="time" className="w-28!" value={w.start} onChange={(e) => setWindow(i, { start: e.target.value })} />
              <span className="text-ink-3">to</span>
              <Input aria-label="End" type="time" className="w-28!" value={w.end} onChange={(e) => setWindow(i, { end: e.target.value })} />
              <Button variant="ghost" className="h-7 px-1.5" aria-label="Remove window" onClick={() => setForm({ ...form, windows: form.windows.filter((_, j) => j !== i) })}><X size={14} /></Button>
            </div>
          ))}
          <Button variant="ghost" className="h-7" onClick={() => setForm({ ...form, windows: [...form.windows, { days: [1, 2, 3, 4, 5], start: '22:00', end: '05:00' }] })}><Plus size={14} aria-hidden /> Add window</Button>
          {errors.windows && <p className="text-[13px] text-danger">{errors.windows}</p>}
        </fieldset>
        <label className="col-span-2 flex items-center gap-2 text-[13px] text-ink"><input type="checkbox" checked={form.isSensitive} onChange={(e) => setForm({ ...form, isSensitive: e.target.checked })} />
          Approved for sensitive networks (core banking, payments). Without this, sensitive subnets get passive methods only.</label>
            </div>
          </MoreOptions>
        </div>
        <div className="col-span-2">{Object.keys(errors).length === 0 && <FormError error={save.error} />}</div>
      </div>
    </Modal>
  )
}

export function Policies() {
  const refresh = useRefresh()
  const canManage = useCan('discovery:manage')
  const [editing, setEditing] = useState<Policy | 'new' | null>(null)
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['discovery', 'policies'], queryFn: () => list<Policy>('/discovery/policies') })
  const approve = useMutation({ meta: { success: 'Policy approved.', inlineError: true }, mutationFn: (id: string) => post(`/discovery/policies/${id}/approve`), onSuccess: refresh })
  return (
    <Card title="Policies" action={canManage && <Button variant="primary" className="h-7" onClick={() => setEditing('new')}><Plus size={14} aria-hidden /> New policy</Button>}>
      <p className="border-b border-line px-4 py-2 text-[12.5px] text-ink-2">A policy says how and where discovery may run. It needs approval by someone other than its author, and any edit needs approval again.</p>
      {isPending && <SkeletonRows rows={4} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && <EmptyState title="No policies yet" />}
      {data && data.data.length > 0 && (
        <table className="w-full border-collapse">
          <thead><tr><Th>Name</Th><Th>Methods</Th><Th>Approved networks</Th><Th>Limits</Th><Th>Approval</Th>{canManage && <Th />}</tr></thead>
          <tbody>{data.data.map((p) => (
            <tr key={p.id}>
              <Td className="font-medium text-ink">{p.name}{p.isSensitive && <span className="ml-1"><Badge tone="warn">Sensitive</Badge></span>}</Td>
              <Td className="text-ink-2">{p.methods.join(', ')}</Td>
              <Td><Mono>{p.targetAllowlist.join(', ')}</Mono></Td>
              <Td className="text-ink-2">{p.maxPps}/s · {p.windows.length ? `${p.windows.length} windows` : 'any time'}</Td>
              <Td>{p.approvedAt ? <Badge tone="ok">Approved by {p.approvedByEmail}</Badge> : <Badge tone="warn">Needs approval</Badge>}</Td>
              {canManage && <Td className="text-right whitespace-nowrap">
                {!p.approvedAt && <Button className="h-7" busy={approve.isPending && approve.variables === p.id} onClick={() => approve.mutate(p.id)}><ShieldCheck size={14} aria-hidden /> Approve</Button>}
                <Button variant="ghost" className="h-7" onClick={() => setEditing(p)}>Edit</Button>
              </Td>}
            </tr>
          ))}</tbody>
        </table>
      )}
      <div className="px-4 py-2"><FormError error={approve.error} /></div>
      {editing && <PolicyModal policy={editing === 'new' ? null : editing} onClose={() => setEditing(null)} />}
    </Card>
  )
}
