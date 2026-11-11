import { PageHeader, PathTabs } from '../../components/ui'
import { useMe } from '../../lib/session'
import { useRouteTab } from '../../lib/tabs'
import { AuditTab } from './AuditTab'
import { EnvironmentsTab } from './EnvironmentsTab'
import { FortiGateTab } from './FortiGateTab'
import { SegmentsTab } from './SegmentsTab'
import { SystemTab } from './SystemTab'
import { UsersTab } from './UsersTab'

const tabs = [
  { key: 'settings', label: 'Segments', permission: 'system:manage', view: SegmentsTab },
  { key: 'environments', label: 'Environments', permission: 'system:manage', view: EnvironmentsTab },
  { key: 'users', label: 'Users', permission: 'users:manage', view: UsersTab },
  { key: 'audit', label: 'Audit log', permission: 'audit:view', view: AuditTab },
  { key: 'system', label: 'System', permission: 'system:manage', view: SystemTab },
  { key: 'integrations', label: 'FortiGate', permission: 'system:manage', view: FortiGateTab },
]

export function SettingsPage() {
  const { permissions } = useMe()
  const allowed = tabs.filter((t) => permissions.includes(t.permission))
  const key = useRouteTab(allowed.map((t) => t.key))
  const Current = allowed.find((t) => t.key === key)?.view
  return (
    <>
      <PageHeader title="Settings" description="Segments, environments, people, the audit trail and how DF-IPAM watches the network." />
      <PathTabs base="/settings" tabs={allowed.map(({ key: k, label }) => ({ key: k, label }))} />
      {Current ? <Current /> : null}
    </>
  )
}
