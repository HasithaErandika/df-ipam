import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { Badge, Button, Card, ErrorState, Field, FormError, Input, Modal, SkeletonRows, Td, Th, useConfirmSave } from '../../components/ui'
import { useChecks } from '../../components/workflow'
import { get, put } from '../../lib/api'
import { classifications } from '../../lib/format'
import { useCan } from '../../lib/session'
import type {  FindingRule } from '../../lib/types'
import { between, issues } from '../../lib/validation'

function RuleEditor({ rule, onClose }: { rule: FindingRule | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState<FindingRule | null>(rule)
  const [priority, setPriority] = useState('')
  useEffect(() => { setForm(rule); setPriority(rule?.sourcePriority.join(', ') ?? '') }, [rule])
  const confirmSave = useConfirmSave()
  const save = useMutation({ meta: { success: 'Rule saved.', inlineError: true },
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
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => checks.submit(() => confirmSave('this rule', () => save.mutate()))}>Save</Button></>}>
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
