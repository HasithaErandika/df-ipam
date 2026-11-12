import { PageHeader, PathTabs } from '../../components/ui'
import { useRouteTab } from '../../lib/tabs'
import { FortiGateTab } from './FortiGateTab'
import { SystemTab } from './SystemTab'

const tabs = [
  { key: 'system', label: 'Settings', view: SystemTab },
  { key: 'fortigate', label: 'FortiGate', view: FortiGateTab },
]

/** How DF-IPAM runs: scan and check intervals, email, sessions and the firewall connection. */
export function SystemPage() {
  const key = useRouteTab(tabs.map((t) => t.key))
  const Current = tabs.find((t) => t.key === key)?.view ?? SystemTab
  return (
    <>
      <PageHeader title="System" description="Discovery and server-check intervals, email, sessions, and the optional read-only FortiGate connection." />
      <PathTabs base="/system" tabs={tabs.map(({ key: k, label }) => ({ key: k, label }))} />
      <Current />
    </>
  )
}
