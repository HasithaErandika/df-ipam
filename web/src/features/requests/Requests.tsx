import { useRouteTab } from '../../lib/tabs'
import { useQuery } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useState } from 'react'
import { useSearchParams } from 'react-router'
import { Button, PageHeader } from '../../components/ui'
import { type TabDef, RouteTabs } from '../../components/workflow'
import { NewRequestModal } from './NewRequestModal'
import { RequestModal } from './RequestModal'
import { AllocationsTable, ApprovalsTable, RequestsTable } from './tables'
import { WorkflowsPanel } from './workflows'
import { list } from '../../lib/api'
import { useCan } from '../../lib/session'
import type { Allocation, PendingApproval } from '../../lib/types'

export function Requests() {
  const canRequest = useCan('ip:request')
  const canApprove = useCan('requests:approve')
  const canAllocate = useCan('ip:allocate')
  const canReserve = useCan('ip:reserve')
  const canAudit = useCan('audit:view')
  const seeAll = canApprove || canAllocate || canReserve || canAudit
  const canManage = useCan('users:manage')
  const [params, setParams] = useSearchParams()
  const [creating, setCreating] = useState(false)
  const [releasing, setReleasing] = useState<Allocation | null>(null)
  const openId = params.get('id')
  const pending = useQuery({ queryKey: ['approvals', 'count'], enabled: canApprove, queryFn: () => list<PendingApproval>('/approvals?pageSize=1') })

  const setParam = (key: string, value: string | null) => setParams((prev) => {
    const next = new URLSearchParams(prev)
    if (value) next.set(key, value)
    else next.delete(key)
    return next
  })
  const tabs: TabDef[] = [
    ...(canApprove ? [{ key: 'approvals', label: 'Waiting for me', count: pending.data?.meta.total }] : []),
    ...(canRequest ? [{ key: 'mine', label: 'My requests' }] : []),
    ...(seeAll ? [{ key: 'all', label: 'All requests' }] : []),
    { key: 'allocations', label: 'Allocations' },
    ...(canApprove || canManage ? [{ key: 'workflows', label: 'Workflows' }] : []),
  ]
  const active = useRouteTab(tabs.map((t) => t.key))

  return (
    <>
      <PageHeader title="IP Allocation Requests" description="Request, approve and fulfil allocations, reservations and releases. Every step is approved by someone else and audited."
        actions={canRequest && <Button variant="primary" onClick={() => setCreating(true)}><Plus size={15} aria-hidden /> New request</Button>} />
      <RouteTabs tabs={tabs} active={active} base="/requests" />
      {active === 'approvals' && <ApprovalsTable onOpen={(id) => setParam('id', id)} />}
      {active === 'mine' && <RequestsTable mine onOpen={(id) => setParam('id', id)} />}
      {active === 'all' && <RequestsTable mine={false} onOpen={(id) => setParam('id', id)} />}
      {active === 'allocations' && <AllocationsTable onOpen={(id) => setParam('id', id)} onRelease={setReleasing} />}
      {active === 'workflows' && <WorkflowsPanel />}
      {creating && <NewRequestModal open onClose={() => setCreating(false)} />}
      {releasing && <NewRequestModal open preset={{ requestType: 'release', allocation: releasing }} onClose={() => setReleasing(null)} />}
      <RequestModal id={openId} onClose={() => setParam('id', null)} />
    </>
  )
}
