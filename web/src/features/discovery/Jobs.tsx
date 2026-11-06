import { keepPreviousData, useMutation, useQuery } from '@tanstack/react-query'
import { Pause, Play, Radar } from 'lucide-react'
import { useState } from 'react'
import { Badge, Button, Card, EmptyState, ErrorState, FormError, Modal, Mono, Pagination, Select, SkeletonRows, Td, Th, useConfirm } from '../../components/ui'
import { Detail } from '../../components/workflow'
import { get, list, post, queryString } from '../../lib/api'
import { formatDateTime } from '../../lib/format'
import { useClampPage, usePaging } from '../../lib/paging'
import { useCan } from '../../lib/session'
import { jobTone, methodLabels, useRefresh } from './shared'
import type {  Job } from './shared'

function JobModal({ id, onClose }: { id: string; onClose: () => void }) {
  const refresh = useRefresh()
  const canRun = useCan('discovery:run')
  const job = useQuery({ queryKey: ['discovery', 'job', id], refetchInterval: 5000, queryFn: () => get<Job>(`/discovery/jobs/${id}`) })
  const logs = useQuery({ queryKey: ['discovery', 'job', id, 'logs'], refetchInterval: 5000, queryFn: () => get<{ level: string; message: string; loggedAt: string }[]>(`/discovery/jobs/${id}/logs`) })
  const confirm = useConfirm()
  const act = useMutation({ meta: { success: (a: string) => `Job ${a === 'pause' ? 'paused' : a === 'resume' ? 'resumed' : 'cancelled'}.`, inlineError: true }, mutationFn: (a: string) => post(`/discovery/jobs/${id}/${a}`), onSuccess: refresh })
  const j = job.data
  return (
    <Modal open onClose={onClose} size="xl" title="Discovery job" subtitle={j && <span className="flex items-center gap-2"><Badge tone={jobTone[j.status]}>{j.status}</Badge> {j.policyName}</span>}
      footer={j && canRun && <>
        {['queued', 'running'].includes(j.status) && <Button busy={act.isPending} onClick={() => act.mutate('pause')}><Pause size={14} aria-hidden /> Pause</Button>}
        {j.status === 'paused' && <Button busy={act.isPending} onClick={() => act.mutate('resume')}><Play size={14} aria-hidden /> Resume</Button>}
        {['queued', 'running', 'paused'].includes(j.status) && <Button busy={act.isPending} onClick={async () => { if (await confirm({ title: 'Cancel this job?', body: 'Discovery on its remaining targets stops. Results already received are kept.', confirmLabel: 'Cancel job' })) act.mutate('cancel') }}>Cancel job</Button>}
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

export function Jobs({ onStart }: { onStart: () => void }) {
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
