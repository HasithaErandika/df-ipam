import { PageHeader, PathTabs } from '../../components/ui'
import { useRouteTab } from '../../lib/tabs'
import { FortiGateTab } from './FortiGateTab'
import { IntuneTab } from './IntuneTab'
import { SystemTab } from './SystemTab'

const tabs = [
  { key: 'system', label: 'Settings', view: SystemTab },
  { key: 'fortigate', label: 'FortiGate', view: FortiGateTab },
  { key: 'intune', label: 'Microsoft Intune', view: IntuneTab },
]

/** How DF-Atlas runs: scan and check intervals, email, sessions, the firewall and Intune connections. */
export function SystemPage() {
  const key = useRouteTab(tabs.map((t) => t.key))
  const Current = tabs.find((t) => t.key === key)?.view ?? SystemTab
  return (
    <>
      <PageHeader title="System" description="Discovery and server-check intervals, email, sessions, and the optional read-only FortiGate and Microsoft Intune connections." />
      <PathTabs base="/system" tabs={tabs.map(({ key: k, label }) => ({ key: k, label }))} />
      <Current />
    </>
  )
}
