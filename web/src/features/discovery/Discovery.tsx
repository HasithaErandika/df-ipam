import { useRouteTab } from '../../lib/tabs'
import { useMutation, useQuery } from '@tanstack/react-query'
import { OctagonX, Radar } from 'lucide-react'
import { useState } from 'react'
import { CatalogManager } from '../../components/catalog/CatalogManager'
import { Button, Field, FormError, Input, Modal, PageHeader, useConfirm } from '../../components/ui'
import { RouteTabs } from '../../components/workflow'
import { Agents } from './Agents'
import { blackouts, exclusions, schedules } from './entities'
import { Jobs } from './Jobs'
import { Policies } from './Policies'
import { useRefresh } from './shared'
import type { Status } from './shared'
import { StartJobModal } from './StartJobModal'
import { fieldIssues, get, post } from '../../lib/api'
import { formatDateTime } from '../../lib/format'
import { useCan } from '../../lib/session'

export function Discovery() {
  const refresh = useRefresh()
  const canRun = useCan('discovery:run')
  const canManage = useCan('discovery:manage')
  const [starting, setStarting] = useState(false)
  const [stopping, setStopping] = useState(false)
  const [reason, setReason] = useState('')
  const status = useQuery({ queryKey: ['discovery', 'status'], refetchInterval: 5000, queryFn: () => get<Status>('/discovery/status') })
  const confirm = useConfirm()
  const stop = useMutation({ meta: { success: (active: boolean) => active ? 'Emergency stop is on. All discovery has stopped.' : 'Emergency stop lifted.', inlineError: true }, mutationFn: (active: boolean) => post('/discovery/emergency-stop', { active, reason }), onSuccess: () => { refresh(); setStopping(false); setReason('') } })
  const tabs = [{ key: 'jobs', label: 'Jobs' }, { key: 'policies', label: 'Policies' }, { key: 'schedules', label: 'Schedules' },
    { key: 'exclusions', label: 'Exclusions' }, { key: 'blackouts', label: 'Blackouts' }, { key: 'agents', label: 'Agents' }]
  const tab = useRouteTab(tabs.map((t) => t.key))
  const s = status.data
  const active = (s?.running ?? 0) + (s?.queued ?? 0) + (s?.paused ?? 0)
  // Only offered when there is something to stop. If the status can't be read, it stays
  // available, because jobs may be running.
  const showStop = canRun && !s?.emergencyStop && (active > 0 || status.isError)
  return (
    <>
      <PageHeader title="Discovery Centre" description="Controlled discovery inside approved scopes. Results become evidence and observation state; they never change allocation or ownership."
        actions={<>
          {canRun && <Button variant="primary" disabled={s?.emergencyStop} onClick={() => setStarting(true)}><Radar size={15} aria-hidden /> Scan subnets</Button>}
        </>} />
      {s?.emergencyStop && (
        <div role="alert" className="mb-3 flex flex-wrap items-center justify-between gap-2 rounded-md border border-danger/40 bg-danger-soft px-4 py-2.5 text-danger">
          <span className="font-medium">Emergency stop is on{s.changedAt && ` since ${formatDateTime(s.changedAt)}`}. No discovery runs anywhere.</span>
          {canManage && <Button onClick={async () => { if (await confirm({ title: 'Lift the emergency stop?', body: 'Discovery can run again. Stopped jobs stay stopped.', confirmLabel: 'Lift the stop', tone: 'primary' })) stop.mutate(false) }} busy={stop.isPending}>Lift the stop</Button>}
        </div>
      )}
      {(s || status.isError) && (
        <div className={`mb-3 flex min-h-11 flex-wrap items-center justify-between gap-3 rounded-md border px-3 py-1.5 text-[13px] text-ink-2 ${active > 0 ? 'border-accent/40 bg-accent-soft/40' : 'border-line bg-surface'}`}>
          {s ? (
            <div className="flex flex-wrap gap-3">
              <span><b className="text-ink">{s.running}</b> running</span><span><b className="text-ink">{s.queued}</b> queued</span><span><b className="text-ink">{s.paused}</b> paused</span>
              <span><b className="text-ink">{s.agentsOnline}</b> of {s.agentsTotal} agents online</span>
            </div>
          ) : <span>Can't read the discovery status. Jobs may still be running.</span>}
          {showStop && <Button variant="danger" className="h-8" onClick={() => { stop.reset(); setStopping(true) }}><OctagonX size={15} aria-hidden /> Emergency stop</Button>}
        </div>
      )}
      <RouteTabs tabs={tabs} active={tab} base="/discovery" />
      {tab === 'jobs' && <Jobs onStart={() => setStarting(true)} />}
      {tab === 'policies' && <Policies />}
      {tab === 'schedules' && <CatalogManager key="schedules" def={schedules} />}
      {tab === 'exclusions' && <CatalogManager key="exclusions" def={exclusions} />}
      {tab === 'blackouts' && <CatalogManager key="blackouts" def={blackouts} />}
      {tab === 'agents' && <Agents />}
      {starting && <StartJobModal onClose={() => setStarting(false)} />}
      <Modal open={stopping} onClose={() => setStopping(false)} title={active > 0 ? `Stop ${active} discovery ${active === 1 ? 'job' : 'jobs'}?` : 'Stop all discovery?'}
        footer={<><Button onClick={() => setStopping(false)}>Cancel</Button><Button variant="danger" busy={stop.isPending} onClick={() => stop.mutate(true)}>Stop everything now</Button></>}>
        <p className="text-[13px] text-ink-2">Every queued, running and paused job stops, and agents halt at their next contact. Lifting the stop needs the manage permission.</p>
        <div className="mt-3"><Field label="Reason" htmlFor="stop-reason" error={fieldIssues(stop.error).reason}><Input id="stop-reason" value={reason} onChange={(e) => setReason(e.target.value)} /></Field></div>
        <div className="mt-3"><FormError error={stop.error} /></div>
      </Modal>
    </>
  )
}
