import { PageHeader, PathTabs } from '../../components/ui'
import { useRouteTab } from '../../lib/tabs'
import { RolesTab } from './RolesTab'
import { UsersTab } from './UsersTab'

const tabs = [
  { key: 'access', label: 'Users', view: UsersTab },
  { key: 'roles', label: 'Roles', view: RolesTab },
]

/** Who may sign in, and what each role may see and do. */
export function AccessPage() {
  const key = useRouteTab(tabs.map((t) => t.key))
  const Current = tabs.find((t) => t.key === key)?.view ?? UsersTab
  return (
    <>
      <PageHeader title="Users and roles" description="Give people a role, and set up what each role may see and do." />
      <PathTabs base="/access" tabs={tabs.map(({ key: k, label }) => ({ key: k, label }))} />
      <Current />
    </>
  )
}
