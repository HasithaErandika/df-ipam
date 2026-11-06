import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus, X } from 'lucide-react'
import { useState } from 'react'
import { Badge, Button, Card, ConfirmDialog, EmptyState, ErrorState, Field, FormError, Input, Modal, Select, SkeletonRows, Td, Th, useConfirmSave } from '../../components/ui'
import { useChecks } from '../../components/workflow'
import { del, list, listAll, post, put, fieldIssues } from '../../lib/api'
import { environments, requestTypes } from '../../lib/format'
import { useCan } from '../../lib/session'
import type { Role,  Workflow } from '../../lib/types'
import { issues as check, required } from '../../lib/validation'
import { useRefresh } from './useRefresh'

const allSensitivities = ['standard', 'sensitive', 'restricted']

function toggle(list: string[], v: string) { return list.includes(v) ? list.filter((x) => x !== v) : [...list, v] }

function Checks({ legend, options, value, onChange }: { legend: string; options: Record<string, string>; value: string[]; onChange: (v: string[]) => void }) {
  return (
    <fieldset>
      <legend className="mb-1 text-[13px] font-medium text-ink">{legend} <span className="font-normal text-ink-3">(none ticked means any)</span></legend>
      <div className="flex flex-wrap gap-x-4 gap-y-1">
        {Object.entries(options).map(([k, label]) => (
          <label key={k} className="flex items-center gap-1.5 text-[13px] text-ink-2">
            <input type="checkbox" checked={value.includes(k)} onChange={() => onChange(toggle(value, k))} /> {label}
          </label>
        ))}
      </div>
    </fieldset>
  )
}

function WorkflowModal({ workflow, open, onClose }: { workflow: Workflow | null; open: boolean; onClose: () => void }) {
  const refresh = useRefresh()
  const queryClient = useQueryClient()
  const roles = useQuery({ queryKey: ['roles'], enabled: open, queryFn: () => listAll<Role>('/roles') })
  const [name, setName] = useState(workflow?.name ?? '')
  const [types, setTypes] = useState<string[]>(workflow?.matchRules.requestTypes ?? [])
  const [envs, setEnvs] = useState<string[]>(workflow?.matchRules.environments ?? [])
  const [sens, setSens] = useState<string[]>(workflow?.matchRules.sensitivities ?? [])
  const [active, setActive] = useState(workflow?.isActive ?? true)
  const [steps, setSteps] = useState<string[]>(workflow?.steps.map((s) => s.approverRoleId) ?? [''])
  const confirmSave = useConfirmSave()
  const save = useMutation({ meta: { success: 'Workflow saved.', inlineError: true },
    mutationFn: () => {
      const body = { name, isActive: active, matchRules: { requestTypes: types, environments: envs, sensitivities: sens }, steps: steps.map((approverRoleId) => ({ approverRoleId })) }
      return workflow ? put(`/approval-workflows/${workflow.id}`, body) : post('/approval-workflows', body)
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['workflows'] }); refresh(); onClose() },
  })
  const checks = useChecks(check({
    name: required(name, 'Name'),
    steps: steps.some((r) => !r) ? 'Choose an approver role for every step.' : undefined,
  }), open)
  const errors = { ...fieldIssues(save.error), ...checks.shown }
  return (
    <Modal open={open} onClose={onClose} title={workflow ? 'Edit workflow' : 'New approval workflow'} size="lg"
      subtitle="The most specific matching workflow applies when a request is submitted. Requests already submitted keep their steps."
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => checks.submit(() => workflow ? confirmSave(`workflow ${workflow.name}`, () => save.mutate()) : save.mutate())}>Save workflow</Button></>}>
      <div className="space-y-4">
        <Field label="Name" htmlFor="wf-name" error={errors.name}><Input id="wf-name" value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Checks legend="Request types" options={requestTypes} value={types} onChange={setTypes} />
        <Checks legend="Environments" options={environments} value={envs} onChange={setEnvs} />
        <Checks legend="Subnet sensitivity" options={Object.fromEntries(allSensitivities.map((s) => [s, s.charAt(0).toUpperCase() + s.slice(1)]))} value={sens} onChange={setSens} />
        <fieldset>
          <legend className="mb-1 text-[13px] font-medium text-ink">Approval steps, in order</legend>
          <div className="space-y-1.5">
            {steps.map((role, i) => (
              <div key={i} className="flex items-center gap-2">
                <span className="w-14 text-[13px] text-ink-2">Step {i + 1}</span>
                <Select aria-label={`Step ${i + 1} approver role`} value={role} onChange={(e) => setSteps(steps.map((r, j) => (j === i ? e.target.value : r)))}>
                  <option value="">Choose the approver role</option>
                  {roles.data?.map((r) => <option key={r.id} value={r.id}>{r.name}</option>)}
                </Select>
                {steps.length > 1 && <Button variant="ghost" className="h-7 px-1.5" aria-label={`Remove step ${i + 1}`} onClick={() => setSteps(steps.filter((_, j) => j !== i))}><X size={14} /></Button>}
              </div>
            ))}
          </div>
          {steps.length < 5 && <Button variant="ghost" className="mt-1.5 h-7" onClick={() => setSteps([...steps, ''])}><Plus size={14} aria-hidden /> Add step</Button>}
          {errors.steps && <p role="alert" className="mt-1 text-[13px] text-danger">{errors.steps}</p>}
          <p className="mt-1.5 text-[12.5px] text-ink-3">Approvers also need the approve permission. Each step needs a different person, and nobody approves their own request.</p>
        </fieldset>
        <label className="flex items-center gap-2 text-[13px] text-ink"><input type="checkbox" checked={active} onChange={(e) => setActive(e.target.checked)} /> Active</label>
        {Object.keys(errors).length === 0 && <FormError error={save.error} />}
      </div>
    </Modal>
  )
}

export function WorkflowsPanel() {
  const canManage = useCan('users:manage')
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<Workflow | null | 'new'>(null)
  const [removing, setRemoving] = useState<Workflow | null>(null)
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['workflows'], queryFn: () => list<Workflow>('/approval-workflows') })
  const remove = useMutation({ meta: { success: 'Workflow deleted.', inlineError: true },
    mutationFn: (id: string) => del(`/approval-workflows/${id}`),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['workflows'] }); setRemoving(null) },
  })
  const rules = (w: Workflow) => {
    const parts = [
      w.matchRules.requestTypes?.map((t) => requestTypes[t]).join(', '),
      w.matchRules.environments?.map((e) => environments[e]).join(', '),
      w.matchRules.sensitivities?.join(', '),
    ].filter(Boolean)
    return parts.length ? parts.join(' · ') : 'Every request'
  }
  return (
    <Card title="Approval workflows" action={canManage && <Button variant="primary" className="h-7" onClick={() => setEditing('new')}><Plus size={14} aria-hidden /> New workflow</Button>}>
      <p className="border-b border-line px-4 py-2 text-[12.5px] text-ink-2">Without a matching workflow, a request needs one approval from anyone with the approve permission.</p>
      {isPending && <SkeletonRows rows={3} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && <EmptyState title="No workflows yet" body="Add one to require particular roles, or several levels, for some requests." />}
      {data && data.data.length > 0 && (
        <table className="w-full border-collapse">
          <thead><tr><Th>Name</Th><Th>Applies to</Th><Th>Steps</Th><Th>Used by</Th><Th>Status</Th>{canManage && <Th />}</tr></thead>
          <tbody>
            {data.data.map((w) => (
              <tr key={w.id}>
                <Td className="font-medium text-ink">{w.name}</Td>
                <Td className="text-ink-2">{rules(w)}</Td>
                <Td className="text-ink-2">{w.steps.map((s) => s.approverRoleName).join(' → ')}</Td>
                <Td className="text-ink-2 tabular-nums">{w.inUse} requests</Td>
                <Td><Badge tone={w.isActive ? 'ok' : 'neutral'}>{w.isActive ? 'Active' : 'Inactive'}</Badge></Td>
                {canManage && (
                  <Td className="text-right whitespace-nowrap">
                    <Button variant="ghost" className="h-7" onClick={() => setEditing(w)}>Edit</Button>
                    {w.inUse === 0 && <Button variant="ghost" className="h-7" onClick={() => setRemoving(w)}>Delete</Button>}
                  </Td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {editing && <WorkflowModal key={editing === 'new' ? 'new' : editing.id} open workflow={editing === 'new' ? null : editing} onClose={() => setEditing(null)} />}
      <ConfirmDialog open={removing !== null} title="Delete this workflow?" body="No request has used it, so nothing else changes."
        confirmLabel="Delete" busy={remove.isPending} error={remove.error} onConfirm={() => removing && remove.mutate(removing.id)} onCancel={() => setRemoving(null)} />
    </Card>
  )
}
