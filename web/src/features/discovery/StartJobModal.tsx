import { useMutation, useQuery } from '@tanstack/react-query'
import { Radar, X } from 'lucide-react'
import { useState } from 'react'
import { SearchSelect } from '../../components/SearchSelect'
import { Button, Field, FormError, Modal, Mono } from '../../components/ui'
import { Detail, SubnetPicker } from '../../components/workflow'
import { fieldIssues,  list, post } from '../../lib/api'
import type { Subnet } from '../../lib/types'
import { methodLabels, useRefresh } from './shared'
import type { Agent, Job, Policy } from './shared'

/** Starts a discovery job. The server freezes the scope and says what it skipped. */
export function StartJobModal({ subnet: initial, onClose }: { subnet?: Subnet | null; onClose: () => void }) {
  const refresh = useRefresh()
  const policies = useQuery({ queryKey: ['discovery', 'policies'], queryFn: () => list<Policy>('/discovery/policies') })
  const agents = useQuery({ queryKey: ['discovery', 'agents'], queryFn: () => list<Agent>('/agents') })
  const [policyId, setPolicyId] = useState('')
  const [subnets, setSubnets] = useState<Subnet[]>(initial ? [initial] : [])
  const [agentId, setAgentId] = useState('')
  const [created, setCreated] = useState<Job | null>(null)
  const start = useMutation({ meta: { success: 'Discovery job started.', inlineError: true },
    mutationFn: () => post<Job>('/discovery/jobs', { policyId, subnetIds: subnets.map((s) => s.id), agentId: agentId || null }),
    onSuccess: (j) => { refresh(); setCreated(j) },
  })
  const errors = fieldIssues(start.error)
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
