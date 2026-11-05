import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Copy, OctagonX, Pause, Play, Plus, Radar, ShieldCheck, X } from 'lucide-react'
import { useState } from 'react'
import { useSearchParams } from 'react-router'
import { CatalogManager, type EntityDef } from '../components/catalog'
import { Badge, Button, Card, ConfirmDialog, EmptyState, ErrorState, Field, FormError, Input, Modal, Mono, PageHeader, Pagination, Select, SkeletonRows, Td, Th, MoreOptions } from '../components/ui'
import { Detail, SubnetPicker, Tabs } from '../components/workflow'
import { ApiError, get, list, post, put, queryString } from '../lib/api'
import { formatDateTime } from '../lib/format'
import { useClampPage, usePaging } from '../lib/paging'
import { useCan } from '../lib/session'
import type { Subnet } from '../lib/types'
import { SearchSelect } from '../components/SearchSelect'
import { useChecks } from '../components/workflow'
import { between, isCIDR, isCron, issues as check, required } from '../lib/validation'

const methodLabels: Record<string, string> = {
  icmp: 'ICMP ping', arp: 'ARP table', ndp: 'IPv6 neighbours (NDP)', dns: 'Reverse DNS (passive)', snmp: 'SNMP', nmap: 'Nmap host discovery (-sn)',
}

interface Window { days: number[]; start: string; end: string }
interface Policy {
  id: string; name: string; description: string; methods: string[]; targetAllowlist: string[]; maxConcurrency: number; maxPps: number
  timeoutMs: number; retries: number; windows: Window[]; isSensitive: boolean; approvedByEmail: string | null; approvedAt: string | null; updatedAt: string
}
interface Job {
  id: string; policyName: string | null; agentName: string | null; status: string; priority: number; progress: number; error: string | null
  config: { methods: string[]; targets: { routingDomainId: string; cidr: string }[]; skipped: string[]; maxPps: number; maxConcurrency: number }
  initiatedByEmail: string | null; scheduled: boolean; subnetName: string | null; targets: number; targetsDone: number; observations: number
  createdAt: string; startedAt: string | null; finishedAt: string | null
}
interface Agent { id: string; name: string; siteName: string | null; status: string; version: string; lastHeartbeatAt: string | null; registrationPending: boolean; createdAt: string }
interface Status { emergencyStop: boolean; changedAt: string | null; queued: number; running: number; agentsOnline: number; agentsTotal: number }

const jobTone: Record<string, 'neutral' | 'accent' | 'ok' | 'warn' | 'danger'> = {
  queued: 'neutral', running: 'accent', paused: 'warn', completed: 'ok', failed: 'danger', cancelled: 'neutral', stopped: 'danger',
}

function issues(error: unknown): Record<string, string> {
  const out: Record<string, string> = {}
  if (error instanceof ApiError) for (const d of error.details) out[d.field] = d.issue
  return out
}

function useRefresh() {
  const queryClient = useQueryClient()
  return () => { for (const k of ['discovery', 'catalog']) queryClient.invalidateQueries({ queryKey: [k] }) }
}

/** Starts a discovery job. The server freezes the scope and says what it skipped. */
export function StartJobModal({ subnet: initial, onClose }: { subnet?: Subnet | null; onClose: () => void }) {
  const refresh = useRefresh()
  const policies = useQuery({ queryKey: ['discovery', 'policies'], queryFn: () => list<Policy>('/discovery/policies') })
  const agents = useQuery({ queryKey: ['discovery', 'agents'], queryFn: () => list<Agent>('/agents') })
  const [policyId, setPolicyId] = useState('')
  const [subnets, setSubnets] = useState<Subnet[]>(initial ? [initial] : [])
  const [agentId, setAgentId] = useState('')
  const [created, setCreated] = useState<Job | null>(null)
  const start = useMutation({
    mutationFn: () => post<Job>('/discovery/jobs', { policyId, subnetIds: subnets.map((s) => s.id), agentId: agentId || null }),
    onSuccess: (j) => { refresh(); setCreated(j) },
  })
  const errors = issues(start.error)
  const approved = policies.data?.data.filter((p) => p.approvedAt) ?? []
  const policy = approved.find((p) => p.id === policyId)
  if (created) {
    return (
      <Modal open onClose={onClose} title="Discovery job queued" footer={<Button variant="primary" onClick={onClose}>Done</Button>}>
        <p className="text-[13px] text-ink-2">An agent picks it up at its next check, inside the policy's time windows.</p>
        <dl className="mt-3">
          <Detail label="Targets">{created.config.targets.map((t) => t.cidr).join(', ')}</Detail>
          <Detail label="Methods">{created.config.methods.map((m) => methodLabels[m] ?? m).join(', ')}</Detail>
          {created.config.skipped.length > 0 && <Detail label="Skipped">{created.config.skipped.join('; ')}</Detail>}
        </dl>
      </Modal>
    )
  }
  return (
    <Modal open onClose={onClose} size="lg" title="Scan subnets" subtitle="Only approved policies run, only inside their approved networks. Exclusions always win, and sensitive networks get passive methods only."
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={start.isPending} disabled={!policyId || subnets.length === 0} onClick={() => start.mutate()}><Radar size={14} aria-hidden /> Start discovery</Button></>}>
      <div className="space-y-3.5">
        <Field label="Policy" htmlFor="job-policy" error={errors.policyId} hint={approved.length === 0 ? 'No approved policy yet. Create one under Policies, and have someone else approve it.' : undefined}>
          <SearchSelect id="job-policy" value={policyId} onChange={setPolicyId} loading={policies.isPending} fetchError={policies.error} onRetry={() => policies.refetch()}
            options={approved.map((p) => ({ value: p.id, label: p.name }))} />
        </Field>
        {policy && (
          <p className="rounded-[5px] bg-surface-2 px-3 py-2 text-[12.5px] text-ink-2">
            {policy.methods.map((m) => methodLabels[m] ?? m).join(', ')} · at most {policy.maxPps} packets/s · allowed: <Mono>{policy.targetAllowlist.join(', ')}</Mono>
          </p>
        )}
        <Field label="Subnets" htmlFor="job-subnet" error={errors.subnetIds} hint="Discovery must be turned on for each subnet. IPv4 subnets larger than /20 must be split; IPv6 subnets are probed only at recorded addresses.">
          <>
            <div className="mb-1.5 flex flex-wrap gap-1">
              {subnets.map((s) => (
                <span key={s.id} className="inline-flex items-center gap-1 rounded-[4px] border border-line bg-surface-2 px-1.5 text-[12.5px]">
                  <Mono>{s.calculated.cidr}</Mono> {s.name}
                  <button type="button" aria-label={`Remove ${s.name}`} className="cursor-pointer text-ink-3 hover:text-danger" onClick={() => setSubnets(subnets.filter((x) => x.id !== s.id))}><X size={12} /></button>
                </span>
              ))}
            </div>
            <SubnetPicker id="job-subnet" value={null} onChange={(s) => s && !subnets.some((x) => x.id === s.id) && setSubnets([...subnets, s])} />
          </>
        </Field>
        <Field label="Agent (optional)" htmlFor="job-agent" hint="Leave empty to let any active agent take it.">
          <SearchSelect id="job-agent" value={agentId} onChange={setAgentId} emptyLabel="Any agent" loading={agents.isPending} fetchError={agents.error} onRetry={() => agents.refetch()}
            options={agents.data?.data.filter((a) => a.status !== 'revoked').map((a) => ({ value: a.id, label: a.name, detail: a.status })) ?? []} />
        </Field>
        {Object.keys(errors).length === 0 && <FormError error={start.error} />}
      </div>
    </Modal>
  )
}

function JobModal({ id, onClose }: { id: string; onClose: () => void }) {
  const refresh = useRefresh()
  const canRun = useCan('discovery:run')
  const job = useQuery({ queryKey: ['discovery', 'job', id], refetchInterval: 5000, queryFn: () => get<Job>(`/discovery/jobs/${id}`) })
  const logs = useQuery({ queryKey: ['discovery', 'job', id, 'logs'], refetchInterval: 5000, queryFn: () => get<{ level: string; message: string; loggedAt: string }[]>(`/discovery/jobs/${id}/logs`) })
  const act = useMutation({ mutationFn: (a: string) => post(`/discovery/jobs/${id}/${a}`), onSuccess: refresh })
  const j = job.data
  return (
    <Modal open onClose={onClose} size="xl" title="Discovery job" subtitle={j && <span className="flex items-center gap-2"><Badge tone={jobTone[j.status]}>{j.status}</Badge> {j.policyName}</span>}
      footer={j && canRun && <>
        {['queued', 'running'].includes(j.status) && <Button busy={act.isPending} onClick={() => act.mutate('pause')}><Pause size={14} aria-hidden /> Pause</Button>}
        {j.status === 'paused' && <Button busy={act.isPending} onClick={() => act.mutate('resume')}><Play size={14} aria-hidden /> Resume</Button>}
        {['queued', 'running', 'paused'].includes(j.status) && <Button busy={act.isPending} onClick={() => act.mutate('cancel')}>Cancel job</Button>}
      </>}>
      {job.isPending && <SkeletonRows rows={5} />}
      {job.error && <ErrorState error={job.error} onRetry={job.refetch} />}
      {j && (
        <div className="grid gap-5 md:grid-cols-2">
          <dl>
            <Detail label="Progress">{j.progress}% · {j.targetsDone} of {j.targets} targets</Detail>
            <Detail label="Results">{j.observations}</Detail>
            <Detail label="Methods">{j.config.methods.map((m) => methodLabels[m] ?? m).join(', ')}</Detail>
            <Detail label="Limits">{j.config.maxPps} packets/s, {j.config.maxConcurrency} at once</Detail>
            <Detail label="Agent">{j.agentName ?? 'Not claimed yet'}</Detail>
            <Detail label="Started by">{j.scheduled ? 'Schedule' : j.initiatedByEmail ?? '—'}</Detail>
            <Detail label="Created">{formatDateTime(j.createdAt)}</Detail>
            {j.finishedAt && <Detail label="Finished">{formatDateTime(j.finishedAt)}</Detail>}
            {j.error && <Detail label="Error"><span className="text-danger">{j.error}</span></Detail>}
            <Detail label="Frozen scope"><Mono>{j.config.targets.map((t) => t.cidr).join(', ')}</Mono></Detail>
            {j.config.skipped.length > 0 && <Detail label="Skipped">{j.config.skipped.join('; ')}</Detail>}
          </dl>
          <section>
            <h3 className="mb-1 text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase">Log</h3>
            <ul className="max-h-80 overflow-y-auto rounded-[5px] border border-line bg-surface-2 p-2 font-mono text-[11.5px]">
              {logs.data?.map((l, i) => (
                <li key={i} className={l.level === 'error' ? 'text-danger' : l.level === 'warning' ? 'text-warn' : 'text-ink-2'}>{new Date(l.loggedAt).toLocaleTimeString('en-GB')} {l.message}</li>
              ))}
              {logs.data?.length === 0 && <li className="text-ink-3">No log lines yet.</li>}
            </ul>
          </section>
        </div>
      )}
      <div className="mt-3"><FormError error={act.error} /></div>
    </Modal>
  )
}

function Jobs({ onStart }: { onStart: () => void }) {
  const canRun = useCan('discovery:run')
  const paging = usePaging('jobs')
  const [status, setStatus] = useState('')
  const [open, setOpen] = useState<string | null>(null)
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, status })
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['discovery', 'jobs', qs], placeholderData: keepPreviousData, refetchInterval: 10000, queryFn: () => list<Job>(`/discovery/jobs${qs}`) })
  useClampPage(paging, data?.meta)
  return (
    <Card title="Jobs" action={canRun && <Button variant="primary" className="h-7" onClick={onStart}><Radar size={14} aria-hidden /> Scan subnets</Button>}>
      <div className="border-b border-line p-2.5">
        <Select aria-label="Status" className="w-40!" value={status} onChange={(e) => { setStatus(e.target.value); paging.setPage(1) }}>
          <option value="">All statuses</option>{Object.keys(jobTone).map((s) => <option key={s} value={s}>{s}</option>)}
        </Select>
      </div>
      {isPending && <SkeletonRows rows={5} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && <EmptyState title="No discovery jobs yet" body="Jobs run only under an approved policy, inside its approved networks." />}
      {data && data.data.length > 0 && (
        <>
          <table className="w-full border-collapse">
            <thead><tr><Th>Started</Th><Th>Policy</Th><Th>Scope</Th><Th>Agent</Th><Th className="text-right">Results</Th><Th>Progress</Th><Th>Status</Th></tr></thead>
            <tbody>{data.data.map((j) => (
              <tr key={j.id} className="cursor-pointer hover:bg-surface-2" onClick={() => setOpen(j.id)}>
                <Td className="whitespace-nowrap text-ink-2">{formatDateTime(j.createdAt)}<span className="block text-[11.5px] text-ink-3">{j.scheduled ? 'Schedule' : j.initiatedByEmail}</span></Td>
                <Td className="font-medium text-ink">{j.policyName}</Td>
                <Td className="text-ink-2">{j.subnetName ?? '—'} <span className="text-ink-3">· {j.targets} targets</span></Td>
                <Td className="text-ink-2">{j.agentName ?? '—'}</Td>
                <Td className="text-right tabular-nums">{j.observations}</Td>
                <Td className="tabular-nums text-ink-2">{j.progress}%</Td>
                <Td><Badge tone={jobTone[j.status]}>{j.status}</Badge></Td>
              </tr>
            ))}</tbody>
          </table>
          <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
        </>
      )}
      {open && <JobModal id={open} onClose={() => setOpen(null)} />}
    </Card>
  )
}

const dayNames = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun']

function PolicyModal({ policy, onClose }: { policy: Policy | null; onClose: () => void }) {
  const refresh = useRefresh()
  const [form, setForm] = useState(() => ({
    name: policy?.name ?? '', description: policy?.description ?? '', methods: policy?.methods ?? ['icmp', 'arp', 'dns'],
    allowlist: policy?.targetAllowlist.join('\n') ?? '', maxPps: String(policy?.maxPps ?? 50), maxConcurrency: String(policy?.maxConcurrency ?? 16),
    timeoutMs: String(policy?.timeoutMs ?? 1000), retries: String(policy?.retries ?? 1), isSensitive: policy?.isSensitive ?? false,
    windows: policy?.windows ?? [] as Window[],
  }))
  const save = useMutation({
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
    maxPps: between(form.maxPps, 1, 1000, 'Packets per second'),
    maxConcurrency: between(form.maxConcurrency, 1, 256, 'Probes at once'),
    timeoutMs: between(form.timeoutMs, 100, 30000, 'Timeout'),
    retries: between(form.retries, 0, 5, 'Retries'),
    windows: form.windows.some((w) => w.days.length === 0 || !w.start || !w.end) ? 'Each window needs at least one day, a start and an end.' : undefined,
  }))
  const errors = { ...issues(save.error), ...checks.shown }
  const setWindow = (i: number, w: Partial<Window>) => setForm((f) => ({ ...f, windows: f.windows.map((x, j) => (j === i ? { ...x, ...w } : x)) }))
  return (
    <Modal open onClose={onClose} size="lg" title={policy ? 'Edit policy' : 'New discovery policy'} subtitle="Saving needs a fresh approval from someone else before the policy runs again."
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => checks.submit(() => save.mutate())}>Save policy</Button></>}>
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

function Policies() {
  const refresh = useRefresh()
  const canManage = useCan('discovery:manage')
  const [editing, setEditing] = useState<Policy | 'new' | null>(null)
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['discovery', 'policies'], queryFn: () => list<Policy>('/discovery/policies') })
  const approve = useMutation({ mutationFn: (id: string) => post(`/discovery/policies/${id}/approve`), onSuccess: refresh })
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

function Agents() {
  const refresh = useRefresh()
  const canManage = useCan('discovery:manage')
  const [creating, setCreating] = useState(false)
  const [name, setName] = useState('')
  const [token, setToken] = useState<{ token: string; agent: string; expiresAt: string } | null>(null)
  const [revoking, setRevoking] = useState<Agent | null>(null)
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['discovery', 'agents'], refetchInterval: 30000, queryFn: () => list<Agent>('/agents') })
  type TokenResponse = { agent: Agent; registrationToken: string; expiresAt: string }
  const show = (r: TokenResponse) => { refresh(); setToken({ token: r.registrationToken, agent: r.agent.name, expiresAt: r.expiresAt }) }
  const create = useMutation({ mutationFn: () => post<TokenResponse>('/agents', { name }), onSuccess: (r) => { setCreating(false); setName(''); show(r) } })
  const rotate = useMutation({ mutationFn: (id: string) => post<TokenResponse>(`/agents/${id}/rotate`), onSuccess: show })
  const revoke = useMutation({ mutationFn: (id: string) => post(`/agents/${id}/revoke`), onSuccess: () => { refresh(); setRevoking(null) } })
  const tone = (s: string) => (s === 'active' ? 'ok' : s === 'revoked' ? 'danger' : s === 'offline' ? 'warn' : 'neutral') as 'ok'
  return (
    <Card title="Agents" action={canManage && <Button variant="primary" className="h-7" onClick={() => setCreating(true)}><Plus size={14} aria-hidden /> New agent</Button>}>
      <p className="border-b border-line px-4 py-2 text-[12.5px] text-ink-2">Agents run inside network segments, pull approved jobs and post results. They never connect to the database.</p>
      {isPending && <SkeletonRows rows={3} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && <EmptyState title="No agents yet" body="Create one, then start the agent with its registration token." />}
      {data && data.data.length > 0 && (
        <table className="w-full border-collapse">
          <thead><tr><Th>Name</Th><Th>Site</Th><Th>Version</Th><Th>Last heartbeat</Th><Th>Status</Th>{canManage && <Th />}</tr></thead>
          <tbody>{data.data.map((a) => (
            <tr key={a.id}>
              <Td className="font-medium text-ink">{a.name}</Td>
              <Td className="text-ink-2">{a.siteName ?? '—'}</Td>
              <Td className="text-ink-2">{a.version || '—'}</Td>
              <Td className="text-ink-2">{a.lastHeartbeatAt ? formatDateTime(a.lastHeartbeatAt) : 'Never'}</Td>
              <Td><Badge tone={tone(a.status)}>{a.status}{a.registrationPending ? ' · token issued' : ''}</Badge></Td>
              {canManage && <Td className="text-right whitespace-nowrap">{a.status !== 'revoked' && <>
                <Button variant="ghost" className="h-7" onClick={() => rotate.mutate(a.id)}>Rotate credential</Button>
                <Button variant="ghost" className="h-7" onClick={() => { revoke.reset(); setRevoking(a) }}>Revoke</Button>
              </>}</Td>}
            </tr>
          ))}</tbody>
        </table>
      )}
      <Modal open={creating} onClose={() => setCreating(false)} title="New agent"
        footer={<><Button onClick={() => setCreating(false)}>Cancel</Button><Button variant="primary" busy={create.isPending} onClick={() => create.mutate()}>Create agent</Button></>}>
        <Field label="Name" htmlFor="agent-name" error={issues(create.error).name}><Input id="agent-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="agent-cmb-dc1" /></Field>
        <div className="mt-3"><FormError error={create.error} /></div>
      </Modal>
      <Modal open={token !== null} onClose={() => setToken(null)} title={`Registration token for ${token?.agent}`} footer={<Button variant="primary" onClick={() => setToken(null)}>I've copied it</Button>}>
        <p className="text-[13px] text-ink-2">Shown once. It works a single time, until {token && formatDateTime(token.expiresAt)}. Start the agent with:</p>
        <pre className="mt-2 overflow-x-auto rounded-[5px] bg-surface-2 p-2 font-mono text-[12px]">AGENT_API_URL={window.location.origin} AGENT_REGISTRATION_TOKEN={token?.token} ./agent</pre>
        <Button className="mt-2" onClick={() => token && navigator.clipboard?.writeText(token.token)}><Copy size={14} aria-hidden /> Copy token</Button>
      </Modal>
      <ConfirmDialog open={revoking !== null} title={`Revoke ${revoking?.name}?`} body="Its credential stops working at once, and its running jobs fail. Create a new agent to replace it."
        confirmLabel="Revoke" busy={revoke.isPending} error={revoke.error} onConfirm={() => revoking && revoke.mutate(revoking.id)} onCancel={() => setRevoking(null)} />
    </Card>
  )
}

const routingDomain = { key: 'routingDomainId', label: 'Routing domain', type: 'ref' as const, ref: '/routing-domains', refLabel: (o: Record<string, string>) => `${o.organisationName} / ${o.name}` }

const exclusions: EntityDef = {
  key: 'exclusions', label: 'Exclusions', singular: 'exclusion', path: '/discovery/exclusions', writePermission: 'discovery:manage',
  description: 'Ranges discovery never touches, whatever a policy allows. Exclusions always win, even on jobs already running.',
  fields: [{ key: 'target', label: 'Network', type: 'cidr', required: true }, { ...routingDomain, hint: 'Leave empty to exclude it in every routing domain.' },
    { key: 'reason', label: 'Reason', type: 'text', required: true }, { key: 'expiresAt', label: 'Expires', type: 'date' }],
  columns: [{ label: 'Network', render: (r) => <Mono>{r.target}</Mono> }, { label: 'Routing domain', render: (r) => r.routingDomainName ?? 'All' },
    { label: 'Reason', render: (r) => r.reason }, { label: 'Expires', render: (r) => (r.expiresAt ? formatDateTime(r.expiresAt) : 'Never') }],
}

const blackouts: EntityDef = {
  key: 'blackouts', label: 'Blackouts', singular: 'blackout', path: '/discovery/blackouts', writePermission: 'discovery:manage', searchLabel: 'Search blackouts',
  description: 'Maintenance periods when no new discovery starts on the network, or on everything if no network is given.',
  fields: [{ key: 'name', label: 'Name', type: 'text', required: true }, { key: 'target', label: 'Network', type: 'cidr' },
    { key: 'startsAt', label: 'Starts', type: 'datetime', required: true, hint: 'In your own time zone.' },
    { key: 'endsAt', label: 'Ends', type: 'datetime', required: true }, { key: 'reason', label: 'Reason', type: 'text' }],
  validate: (f) => ({ endsAt: f.startsAt && f.endsAt && String(f.endsAt) <= String(f.startsAt) ? 'The end must be after the start.' : undefined }),
  columns: [{ label: 'Name', render: (r) => r.name }, { label: 'Network', render: (r) => (r.target ? <Mono>{r.target}</Mono> : 'Everything') },
    { label: 'From', render: (r) => formatDateTime(r.startsAt) }, { label: 'To', render: (r) => formatDateTime(r.endsAt) }],
}

const schedules: EntityDef = {
  key: 'schedules', label: 'Schedules', singular: 'schedule', path: '/discovery/schedules', writePermission: 'discovery:manage',
  description: 'Recurring discovery of one subnet under an approved policy. The worker starts a job when the schedule matches, unless the last one is still running or the emergency stop is on.',
  fields: [{ key: 'policyId', label: 'Policy', type: 'ref', ref: '/discovery/policies', required: true },
    { key: 'subnetId', label: 'Subnet', type: 'ref', ref: '/subnets', required: true, refLabel: (o) => `${o.calculated?.cidr ?? ''} ${o.name}` },
    { key: 'cron', label: 'Schedule (cron)', type: 'text', required: true, hint: 'minute hour day month weekday, such as 30 23 * * 1-5',
      check: (v) => (isCron(v) ? undefined : 'Use five fields: minute hour day month weekday.') },
    { key: 'timezone', label: 'Timezone', type: 'timezone', hint: 'Defaults to Asia/Colombo.' },
    { key: 'agentId', label: 'Agent', type: 'ref', ref: '/agents' }, { key: 'priority', label: 'Priority (1 high to 9 low)', type: 'number', min: 1, max: 9 },
    { key: 'isEnabled', label: 'Enabled', type: 'bool' }],
  columns: [{ label: 'Policy', render: (r) => r.policyName }, { label: 'Subnet', render: (r) => r.subnetName }, { label: 'Cron', render: (r) => <Mono>{r.cron}</Mono> },
    { label: 'Last run', render: (r) => (r.lastRunAt ? formatDateTime(r.lastRunAt) : 'Never') }, { label: 'Enabled', render: (r) => (r.isEnabled ? <Badge tone="ok">On</Badge> : <Badge>Off</Badge>) }],
}

export function Discovery() {
  const refresh = useRefresh()
  const canRun = useCan('discovery:run')
  const canManage = useCan('discovery:manage')
  const [params, setParams] = useSearchParams()
  const [starting, setStarting] = useState(false)
  const [stopping, setStopping] = useState(false)
  const [reason, setReason] = useState('')
  const status = useQuery({ queryKey: ['discovery', 'status'], refetchInterval: 15000, queryFn: () => get<Status>('/discovery/status') })
  const stop = useMutation({ mutationFn: (active: boolean) => post('/discovery/emergency-stop', { active, reason }), onSuccess: () => { refresh(); setStopping(false); setReason('') } })
  const tabs = [{ key: 'jobs', label: 'Jobs' }, { key: 'policies', label: 'Policies' }, { key: 'schedules', label: 'Schedules' },
    { key: 'exclusions', label: 'Exclusions' }, { key: 'blackouts', label: 'Blackouts' }, { key: 'agents', label: 'Agents' }]
  const tab = tabs.some((t) => t.key === params.get('tab')) ? params.get('tab')! : 'jobs'
  const s = status.data
  return (
    <>
      <PageHeader title="Discovery Centre" description="Controlled discovery inside approved scopes. Results become evidence and observation state; they never change allocation or ownership."
        actions={<>
          {canRun && s && !s.emergencyStop && <Button variant="danger" onClick={() => { stop.reset(); setStopping(true) }}><OctagonX size={15} aria-hidden /> Emergency stop</Button>}
          {canRun && <Button variant="primary" disabled={s?.emergencyStop} onClick={() => setStarting(true)}><Radar size={15} aria-hidden /> Scan subnets</Button>}
        </>} />
      {s?.emergencyStop && (
        <div role="alert" className="mb-3 flex flex-wrap items-center justify-between gap-2 rounded-md border border-danger/40 bg-danger-soft px-4 py-2.5 text-danger">
          <span className="font-medium">Emergency stop is on{s.changedAt && ` since ${formatDateTime(s.changedAt)}`}. No discovery runs anywhere.</span>
          {canManage && <Button onClick={() => stop.mutate(false)} busy={stop.isPending}>Lift the stop</Button>}
        </div>
      )}
      {s && (
        <div className="mb-3 flex flex-wrap gap-3 text-[13px] text-ink-2">
          <span><b className="text-ink">{s.running}</b> running</span><span><b className="text-ink">{s.queued}</b> queued</span>
          <span><b className="text-ink">{s.agentsOnline}</b> of {s.agentsTotal} agents online</span>
        </div>
      )}
      <Tabs tabs={tabs} active={tab} onChange={(key) => setParams({ tab: key })} />
      {tab === 'jobs' && <Jobs onStart={() => setStarting(true)} />}
      {tab === 'policies' && <Policies />}
      {tab === 'schedules' && <CatalogManager key="schedules" def={schedules} />}
      {tab === 'exclusions' && <CatalogManager key="exclusions" def={exclusions} />}
      {tab === 'blackouts' && <CatalogManager key="blackouts" def={blackouts} />}
      {tab === 'agents' && <Agents />}
      {starting && <StartJobModal onClose={() => setStarting(false)} />}
      <Modal open={stopping} onClose={() => setStopping(false)} title="Stop all discovery?"
        footer={<><Button onClick={() => setStopping(false)}>Cancel</Button><Button variant="danger" busy={stop.isPending} onClick={() => stop.mutate(true)}>Stop everything now</Button></>}>
        <p className="text-[13px] text-ink-2">Every queued, running and paused job stops, and agents halt at their next contact. Lifting the stop needs the manage permission.</p>
        <div className="mt-3"><Field label="Reason" htmlFor="stop-reason" error={issues(stop.error).reason}><Input id="stop-reason" value={reason} onChange={(e) => setReason(e.target.value)} /></Field></div>
        <div className="mt-3"><FormError error={stop.error} /></div>
      </Modal>
    </>
  )
}

