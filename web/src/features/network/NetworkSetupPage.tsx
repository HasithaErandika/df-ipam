import { PageHeader, PathTabs } from '../../components/ui'
import { useRouteTab } from '../../lib/tabs'
import { DepartmentsTab } from './DepartmentsTab'
import { EnvironmentsTab } from './EnvironmentsTab'
import { SegmentsTab } from './SegmentsTab'

const tabs = [
  { key: 'network-setup', label: 'Segments', view: SegmentsTab },
  { key: 'environments', label: 'Environments', view: EnvironmentsTab },
  { key: 'departments', label: 'Departments', view: DepartmentsTab },
]

/** The shape of the network: segments, the environments servers belong to, and departments. */
export function NetworkSetupPage() {
  const key = useRouteTab(tabs.map((t) => t.key))
  const Current = tabs.find((t) => t.key === key)?.view ?? SegmentsTab
  return (
    <>
      <PageHeader title="Network setup" description="Segments (DHCP, server and cloud networks), the environments servers belong to, and departments." />
      <PathTabs base="/network-setup" tabs={tabs.map(({ key: k, label }) => ({ key: k, label }))} />
      <Current />
    </>
  )
}
