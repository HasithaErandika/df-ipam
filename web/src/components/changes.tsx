import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { Badge, Button, Card, EmptyState, ErrorState, Field, FormError, Input, Modal, Pagination, Select, SkeletonRows, Td, Th } from './ui'
import { get, list, post, put, queryString } from '../lib/api'
import { classifications, formatDateTime } from '../lib/format'
import { useClampPage, usePaging } from '../lib/paging'
import { useCan, useMe } from '../lib/session'
import { between, issues, optionalMAC, required } from '../lib/validation'
import { PersonSelect } from './SearchSelect'
import { useChecks } from './workflow'
import type { ChangeRequest, Finding, FindingRule } from '../lib/types'

export const changeableTypes = ['ip_address', 'subnet', 'device']

type FieldKind = 'text' | 'person'

/** What each record type allows. Allocation state isn't here: it moves only through the allocation workflow. */
const fields: Record<string, { key: string; label: string; kind: FieldKind }[]> = {
  ip_address: [{ key: 'ownerId', label: 'Owner', kind: 'person' }, { key: 'hostname', label: 'Hostname', kind: 'text' }, { key: 'notes', label: 'Notes', kind: 'text' }],
  subnet: [{ key: 'technicalOwnerId', label: 'Technical owner', kind: 'person' }, { key: 'businessOwnerId', label: 'Business owner', kind: 'person' }],
  device: [{ key: 'ownerId', label: 'Owner', kind: 'person' }, { key: 'notes', label: 'Notes', kind: 'text' }],
}

const label = (key: string) => key.replace(/Id$/, '').replace(/([A-Z])/g, ' $1').toLowerCase()

export function ProposeChange({ finding, onDone }: { finding: Finding; onDone: () => void }) {
  const queryClient = useQueryClient()
  const [values, setValues] = useState<Record<string, string>>({})
  const [reason, setReason] = useState('')
  const propose = useMutation({
    mutationFn: () => post(`/reconciliation/results/${finding.id}/change-requests`, {
      changes: Object.fromEntries(Object.entries(values).filter(([, v]) => v !== '')), reason,
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['findings'] })
      queryClient.invalidateQueries({ queryKey: ['changes'] })
      onDone()
    },
  })
  const set = (key: string, value: string) => setValues((v) => ({ ...v, [key]: value }))
  const checks = useChecks(issues({
    changes: Object.values(values).some((v) => v.trim()) ? undefined : 'Propose at least one change.',
    reason: required(reason, 'Reason'),
    mac: optionalMAC(values.mac ?? ''),
  }))
  return (
    <div className="mt-4 rounded-md border border-line p-3">
      <h3 className="mb-2 text-[13px] font-semibold text-ink">Propose a change</h3>
      <div className="grid gap-2 sm:grid-cols-2">
        {fields[finding.objectType].map((f) => (
          <Field key={f.key} label={f.label} htmlFor={`change-${f.key}`}>
            {f.kind === 'person' ? (
              <PersonSelect id={`change-${f.key}`} value={values[f.key] ?? ''} onChange={(v) => set(f.key, v)} emptyLabel="No change" />
            ) : <Input id={`change-${f.key}`} value={values[f.key] ?? ''} onChange={(e) => set(f.key, e.target.value)} />}
          </Field>
        ))}
      </div>
      <div className="mt-2"><Field label="Reason" htmlFor="change-reason" error={checks.shown.reason}><Input id="change-reason" value={reason} onChange={(e) => setReason(e.target.value)} /></Field></div>
      {checks.shown.changes && <p role="alert" className="mt-2 text-[13px] text-danger">{checks.shown.changes}</p>}
      <div className="mt-2"><FormError error={propose.error} /></div>
      <div className="mt-2 flex justify-end"><Button busy={propose.isPending} onClick={() => checks.submit(() => propose.mutate())}>Send for approval</Button></div>
    </div>
  )
}

const tones = { pending: 'warn', approved: 'accent', applied: 'ok', rejected: 'neutral' } as const

function Decision({ change, onClose }: { change: ChangeRequest | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const me = useMe()
  const canDecide = useCan('requests:approve')
  const [note, setNote] = useState('')
  const [noteMissing, setNoteMissing] = useState(false)
  const decide = useMutation({
    mutationFn: (action: 'approve' | 'reject') => post(`/change-requests/${change?.id}/${action}`, { note }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['changes'] })
      queryClient.invalidateQueries({ queryKey: ['findings'] })
      setNote('')
      onClose()
    },
  })
  if (!change) return null
  const own = change.requestedById === me.id
  const open = change.status === 'pending' && canDecide && !own
  return (
    <Modal open onClose={onClose} title="Change request" subtitle={`${change.objectType.replaceAll('_', ' ')} · ${change.requestedBy ?? 'Unknown'}`} size="lg"
      footer={open ? <><Button onClick={onClose}>Close</Button><Button busy={decide.isPending} onClick={() => (note.trim() ? decide.mutate('reject') : setNoteMissing(true))}>Reject</Button><Button variant="primary" busy={decide.isPending} onClick={() => decide.mutate('approve')}>Approve and apply</Button></> : undefined}>
      {change.finding && <p className="text-ink">{change.finding}</p>}
      <p className="mt-2 text-[13px] text-ink-2">Reason: {change.reason}</p>
      <table className="mt-3 w-full border-collapse">
        <thead><tr><Th>Field</Th><Th>Proposed value</Th></tr></thead>
        <tbody>{Object.entries(change.proposed).map(([k, v]) => <tr key={k}><Td className="text-ink-2">{label(k)}</Td><Td>{v || 'Cleared'}</Td></tr>)}</tbody>
      </table>
      {change.decidedBy && <p className="mt-3 text-[13px] text-ink-2">Decided by {change.decidedBy}{change.decisionNote ? `: ${change.decisionNote}` : ''}</p>}
      {own && change.status === 'pending' && <p className="mt-3 text-[13px] text-ink-2">Someone else must decide a change you proposed.</p>}
      {open && <div className="mt-3"><Field label="Decision note (needed to reject)" htmlFor="change-note" error={noteMissing && !note.trim() ? 'Say why this change is rejected.' : undefined}><Input id="change-note" value={note} onChange={(e) => setNote(e.target.value)} /></Field></div>}
      <div className="mt-2"><FormError error={decide.error} /></div>
    </Modal>
  )
}

export function ChangeRequests() {
  const paging = usePaging()
  const [status, setStatus] = useState('pending')
  const [selected, setSelected] = useState<ChangeRequest | null>(null)
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, status })
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['changes', qs], placeholderData: keepPreviousData, queryFn: () => list<ChangeRequest>(`/change-requests${qs}`) })
  useClampPage(paging, data?.meta)
  return (
    <Card>
      <div className="border-b border-line p-2.5">
        <Select aria-label="Status" className="w-44!" value={status} onChange={(e) => { setStatus(e.target.value); paging.setPage(1) }}>
          <option value="pending">Waiting for a decision</option><option value="applied">Applied</option><option value="rejected">Rejected</option><option value="">All</option>
        </Select>
      </div>
      {isPending && <SkeletonRows rows={6} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && <EmptyState title="No change requests here" body="Propose one from a finding in the Findings tab." />}
      {data && data.data.length > 0 && (
        <>
          <div className="overflow-x-auto">
            <table className="w-full border-collapse">
              <thead><tr><Th>Record</Th><Th>Changes</Th><Th>Proposed by</Th><Th>Proposed</Th><Th>Status</Th></tr></thead>
              <tbody>
                {data.data.map((c) => (
                  <tr key={c.id} className="hover:bg-surface-2">
                    <Td><button type="button" className="cursor-pointer font-medium text-ink hover:text-accent" onClick={() => setSelected(c)}>{c.objectType.replaceAll('_', ' ')}</button></Td>
                    <Td className="text-ink-2">{Object.keys(c.proposed).map(label).join(', ')}</Td>
                    <Td className="text-ink-2">{c.requestedBy ?? '—'}</Td>
                    <Td className="whitespace-nowrap text-ink-2">{formatDateTime(c.createdAt)}</Td>
                    <Td><Badge tone={tones[c.status]}>{c.status}</Badge></Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
        </>
      )}
      <Decision change={selected} onClose={() => setSelected(null)} />
    </Card>
  )
}

function RuleEditor({ rule, onClose }: { rule: FindingRule | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState<FindingRule | null>(rule)
  const [priority, setPriority] = useState('')
  useEffect(() => { setForm(rule); setPriority(rule?.sourcePriority.join(', ') ?? '') }, [rule])
  const save = useMutation({
    mutationFn: () => put(`/reconciliation/rules/${form?.classification}`, {
      ...form, sourcePriority: priority.split(',').map((s) => s.trim()).filter(Boolean),
    }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['finding-rules'] }); onClose() },
  })
  const w = form?.weights
  const used = Boolean(w && (w.reliability || w.freshness || w.agreement || w.cap || w.freshnessDays))
  const checks = useChecks(!w || !used ? {} : issues({
    reliability: Math.abs(w.reliability + w.freshness + w.agreement - 1) > 0.001 ? 'The three weights must add up to 1.' : undefined,
    cap: between(w.cap, 1, 100, 'Highest confidence'),
    freshnessDays: between(w.freshnessDays, 1, 365, 'Days'),
  }))
  if (!form) return null
  const weight = (key: keyof FindingRule['weights'], text: string) => setForm({ ...form, weights: { ...form.weights, [key]: Number(text) } })
  const number = (key: keyof FindingRule['weights'], name: string, step: string) => (
    <Field label={name} htmlFor={`rule-${key}`} error={checks.shown[key]}><Input id={`rule-${key}`} type="number" step={step} value={form.weights[key]} onChange={(e) => weight(key, e.target.value)} /></Field>
  )
  return (
    <Modal open onClose={onClose} title={classifications[form.classification] ?? form.classification} size="lg"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => checks.submit(() => save.mutate())}>Save</Button></>}>
      <label className="flex items-center gap-2 text-ink"><input type="checkbox" className="size-4 accent-[var(--accent)]" checked={form.isEnabled} onChange={(e) => setForm({ ...form, isEnabled: e.target.checked })} /> Detect this finding</label>
      <div className="mt-3"><Field label="Source priority, best first" htmlFor="rule-priority" hint="Comma separated. The first source a finding has sets its reliability."><Input id="rule-priority" value={priority} onChange={(e) => setPriority(e.target.value)} /></Field></div>
      <p className="mt-3 text-[13px] text-ink-2">The three weights must add up to 1. Set everything to 0 to keep the rule's fixed confidence.</p>
      <div className="mt-2 grid gap-2 sm:grid-cols-3">
        {number('reliability', 'Reliability weight', '0.05')}{number('freshness', 'Freshness weight', '0.05')}{number('agreement', 'Agreement weight', '0.05')}
        {number('cap', 'Highest confidence', '1')}{number('freshnessDays', 'Evidence goes stale after (days)', '1')}
      </div>
      <div className="mt-3"><FormError error={save.error} /></div>
    </Modal>
  )
}

export function FindingRules() {
  const canManage = useCan('discovery:manage')
  const [selected, setSelected] = useState<FindingRule | null>(null)
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['finding-rules'], queryFn: () => get<FindingRule[]>('/reconciliation/rules') })
  return (
    <Card>
      {isPending && <SkeletonRows rows={8} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data && (
        <div className="overflow-x-auto">
          <table className="w-full border-collapse">
            <thead><tr><Th>Finding</Th><Th>Detected</Th><Th>Confidence</Th><Th>Top source</Th></tr></thead>
            <tbody>
              {data.map((r) => (
                <tr key={r.classification} className="hover:bg-surface-2">
                  <Td>{canManage ? <button type="button" className="cursor-pointer font-medium text-ink hover:text-accent" onClick={() => setSelected(r)}>{classifications[r.classification] ?? r.classification}</button> : (classifications[r.classification] ?? r.classification)}</Td>
                  <Td><Badge tone={r.isEnabled ? 'ok' : 'neutral'}>{r.isEnabled ? 'On' : 'Off'}</Badge></Td>
                  <Td className="text-ink-2">{r.weights.cap ? `Calculated, up to ${r.weights.cap}%` : 'Fixed in the rule'}</Td>
                  <Td className="text-ink-2">{r.sourcePriority[0] ?? '—'}</Td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <RuleEditor rule={selected} onClose={() => setSelected(null)} />
    </Card>
  )
}

