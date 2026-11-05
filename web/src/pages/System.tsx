import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { Link } from 'react-router'
import { Badge, Button, Card, EmptyState, ErrorState, Field, FormError, Input, Modal, PageHeader, SkeletonRows, Td, Th } from '../components/ui'
import { get, put } from '../lib/api'
import { formatCount, formatDateTime } from '../lib/format'
import { useCan } from '../lib/session'
import type { RetentionPolicy, SystemHealth } from '../lib/types'
import { useChecks } from '../components/workflow'
import { between, issues as check } from '../lib/validation'

const classNames: Record<string, string> = {
  discovery_observations: 'Discovery evidence', discovery_job_logs: 'Discovery job logs', agent_heartbeats: 'Agent heartbeats',
  notifications: 'Notifications', report_jobs: 'Generated reports', audit_logs: 'Audit logs',
}

function Line({ label, children }: { label: string; children: React.ReactNode }) {
  return <div className="flex items-center justify-between gap-3 border-b border-line px-4 py-2 text-[13px] last:border-0"><span className="text-ink-2">{label}</span><span className="text-right text-ink">{children}</span></div>
}

function Health() {
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['system-health'], refetchInterval: 30000, queryFn: () => get<SystemHealth>('/system/health') })
  if (isPending) return <Card title="System health"><SkeletonRows rows={6} /></Card>
  if (error) return <Card title="System health"><ErrorState error={error} onRetry={refetch} /></Card>
  return (
    <div className="grid gap-3 md:grid-cols-2">
      <Card title="Services">
        <Line label="Database"><Badge tone="ok">Connected</Badge></Line>
        <Line label="Worker"><Badge tone={data.workerHealthy ? 'ok' : 'danger'}>{data.workerHealthy ? 'Running' : 'Not running'}</Badge></Line>
        <Line label="Last worker heartbeat">{data.workerHeartbeat ? formatDateTime(data.workerHeartbeat) : 'None recorded'}</Line>
        <Line label="Credential encryption"><Badge tone={data.encryptionConfigured ? 'ok' : 'warn'}>{data.encryptionConfigured ? 'Key loaded' : 'No key set'}</Badge></Line>
        <Line label="Discovery emergency stop"><Badge tone={data.emergencyStop ? 'danger' : 'ok'}>{data.emergencyStop ? 'On' : 'Off'}</Badge></Line>
        <Line label="Last successful backup">{data.lastBackup ? formatDateTime(data.lastBackup) : 'None reported'}</Line>
      </Card>
      <Card title="Queues and connections">
        <Line label="Discovery jobs queued">{formatCount(data.discoveryQueued)}</Line>
        <Line label="Integration syncs queued">{formatCount(data.syncsQueued)}</Line>
        <Line label="Reports queued">{formatCount(data.reportsQueued)}</Line>
        <Line label="Emails waiting">{formatCount(data.emailPending)}</Line>
        <Line label="Agents online / offline">{formatCount(data.agentsOnline)} / {formatCount(data.agentsOffline)}</Line>
        <Line label="Integrations healthy / failing">{formatCount(data.integrationsHealthy)} / {formatCount(data.integrationsFailing)}</Line>
      </Card>
    </div>
  )
}

function EditRetention({ policy, onClose }: { policy: RetentionPolicy | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [days, setDays] = useState(0)
  useEffect(() => { setDays(policy?.retainDays ?? 0) }, [policy])
  const save = useMutation({
    mutationFn: () => put(`/system/retention/${policy?.dataClass}`, { retainDays: days }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['retention'] }); onClose() },
  })
  const checks = useChecks(check({ days: between(days, policy?.dataClass === 'audit_logs' ? 365 : 7, 36500, 'Days') }))
  if (!policy) return null
  return (
    <Modal open onClose={onClose} title={classNames[policy.dataClass] ?? policy.dataClass}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => checks.submit(() => save.mutate())}>Save</Button></>}>
      <Field label="Keep for (days)" htmlFor="retain-days" error={checks.shown.days}><Input id="retain-days" type="number" min={7} value={days} onChange={(e) => setDays(Number(e.target.value))} /></Field>
      <div className="mt-3"><FormError error={save.error} /></div>
    </Modal>
  )
}

function Retention() {
  const canManage = useCan('system:manage')
  const [selected, setSelected] = useState<RetentionPolicy | null>(null)
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['retention'], queryFn: () => get<RetentionPolicy[]>('/system/retention') })
  return (
    <Card title="Retention">
      {isPending && <SkeletonRows rows={5} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.length === 0 && <EmptyState title="No retention policies" body="Run the seed to load the starting values." />}
      {data && data.length > 0 && (
        <table className="w-full border-collapse">
          <thead><tr><Th>Data</Th><Th className="text-right">Keep for</Th><Th>Last changed</Th></tr></thead>
          <tbody>
            {data.map((p) => (
              <tr key={p.dataClass} className="hover:bg-surface-2">
                <Td>{canManage ? <button type="button" className="cursor-pointer font-medium text-ink hover:text-accent" onClick={() => setSelected(p)}>{classNames[p.dataClass] ?? p.dataClass}</button> : (classNames[p.dataClass] ?? p.dataClass)}</Td>
                <Td className="text-right text-ink-2 tabular-nums">{formatCount(p.retainDays)} days</Td>
                <Td className="text-ink-2">{p.updatedBy ? `${p.updatedBy}, ${p.updatedAt ? formatDateTime(p.updatedAt) : ''}` : 'Starting value'}</Td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <p className="border-t border-line px-4 py-2 text-[12px] text-ink-3">These are starting values for review, not agreed policy. Audit logs aren't pruned automatically until compliance sets a period.</p>
      <EditRetention policy={selected} onClose={() => setSelected(null)} />
    </Card>
  )
}

export function System() {
  const canManage = useCan('system:manage')
  const canAudit = useCan('audit:view')
  const canView = canManage || canAudit
  if (!canView) return <><PageHeader title="System Administration" /><Card><EmptyState title="You don't have access to system administration" /></Card></>
  return (
    <>
      <PageHeader title="System Administration" description="Platform health and data retention." />
      <div className="space-y-3">
        <Health />
        <Retention />
        <Card title="Other settings">
          <ul className="divide-y divide-line text-[13px]">
            <li className="px-4 py-2"><Link className="text-accent hover:underline" to="/network">Sites, routing domains, VLANs and VDOMs</Link></li>
            <li className="px-4 py-2"><Link className="text-accent hover:underline" to="/imports">Imports and enrichment queue</Link></li>
            <li className="px-4 py-2"><Link className="text-accent hover:underline" to="/alerts?tab=rules">Notification rules</Link></li>
            <li className="px-4 py-2"><Link className="text-accent hover:underline" to="/roles">Roles and permissions</Link></li>
          </ul>
        </Card>
      </div>
    </>
  )
}
