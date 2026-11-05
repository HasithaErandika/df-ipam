import { ChangeRequests } from './ChangeRequests'
import { FindingRules } from './FindingRules'
import { Findings } from './Findings'
import { PageHeader } from '../../components/ui'
import { RouteTabs } from '../../components/workflow'
import { useRouteTab } from '../../lib/tabs'

const tabs = [{ key: 'findings', label: 'Findings' }, { key: 'changes', label: 'Change requests' }, { key: 'rules', label: 'Rules and confidence' }]

export function Reconciliation() {
  const tab = useRouteTab(tabs.map((t) => t.key))
  return (
    <>
      <PageHeader title="Reconciliation Centre" description="Differences between inventory and evidence. Nothing here changes inventory until a person acts." />
      <RouteTabs tabs={tabs} active={tab} base="/reconciliation" />
      {tab === 'findings' && <Findings />}
      {tab === 'changes' && <ChangeRequests />}
      {tab === 'rules' && <FindingRules />}
    </>
  )
}
