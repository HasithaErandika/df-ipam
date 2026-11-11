import { useMutation } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { Badge, Button, Card, DataTable, Field, FilterBar, FilterSelect, FormError, Input, Modal, none, SearchInput, Select, Stack, Switch, useConfirmSave, type Column } from '../../components/ui'
import { put } from '../../lib/api'
import { formatAgo, formatDateTime } from '../../lib/format'
import { useFilters, usePagedList } from '../../lib/listing'
import { useInvalidate } from '../../lib/queries'
import { useMe } from '../../lib/session'
import { roleLabels } from '../../lib/types/core'
import type { User } from '../../lib/types'

const roleHelp: Record<string, string> = {
  admin: 'Changes the register, approves requests and manages settings.',
  requester: 'Reads everything, raises IP requests and reservations.',
  viewer: 'Reads only.',
  '': 'Signed in but sees nothing until given a role.',
}

function EditUser({ user, onClose }: { user: User | null; onClose: () => void }) {
  const me = useMe()
  const [role, setRole] = useState('')
  const [active, setActive] = useState(true)
  const [name, setName] = useState('')
  const refresh = useInvalidate('/users')
  const confirmSave = useConfirmSave()
  useEffect(() => { if (user) { setRole(user.role ?? ''); setActive(user.isActive); setName(user.displayName) } }, [user])
  const save = useMutation({
    mutationFn: () => put(`/users/${user!.id}`, { role, isActive: active, displayName: name }),
    meta: { inlineError: true, success: 'Access updated. It applies on their next request.' }, onSuccess: () => { void refresh(); onClose() },
  })
  const self = user?.id === me.id
  return (
    <Modal open={user !== null} onClose={onClose} title="Edit access" subtitle={user?.email}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => void confirmSave(user!.email, () => save.mutate(), 'Role and access changes apply immediately and are recorded in the audit log.')}>Save changes</Button></>}>
      <div className="space-y-4">
        <Field label="Display name" htmlFor="u-name"><Input id="u-name" value={name} maxLength={120} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="Role" htmlFor="u-role" hint={self ? "You can't change your own role." : roleHelp[role]}>
          <Select id="u-role" value={role} disabled={self} onChange={(e) => setRole(e.target.value)}>
            <option value="">No role</option>{Object.entries(roleLabels).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </Select>
        </Field>
        <Switch id="u-active" checked={active} disabled={self} onChange={setActive} label="Can sign in" hint="Turning this off ends their access at once. Accounts are never deleted, so the audit trail stays complete." />
        <FormError error={save.error} />
      </div>
    </Modal>
  )
}

export function UsersTab() {
  const f = useFilters(['role', 'active', 'q'] as const)
  const { query, rows, meta, paging } = usePagedList<User>('/users', f.values)
  const [editing, setEditing] = useState<User | null>(null)
  const columns: Column<User>[] = [
    { key: 'who', header: 'Person', cell: (u) => <Stack main={u.displayName || u.email} sub={u.displayName ? u.email : undefined} /> },
    { key: 'role', header: 'Role', cell: (u) => (u.role ? <Badge tone={u.role === 'admin' ? 'accent' : 'neutral'}>{roleLabels[u.role]}</Badge> : <Badge tone="warn">Waiting for a role</Badge>) },
    { key: 'active', header: 'Access', cell: (u) => (u.isActive ? <Badge tone="ok">Active</Badge> : <Badge tone="danger">Deactivated</Badge>) },
    { key: 'login', header: 'Last sign-in', hideBelow: 'md', cell: (u) => (u.lastLoginAt ? <span title={formatDateTime(u.lastLoginAt)}>{formatAgo(u.lastLoginAt)}</span> : none) },
    { key: 'act', header: '', className: 'text-right', cell: (u) => <Button onClick={() => setEditing(u)}>Edit</Button> },
  ]
  return (
    <Card title="Users" subtitle="People appear here after they first sign in. Give each a role.">
      <FilterBar active={f.active} onClear={f.clear}>
        <SearchInput value={f.values.q} onChange={(v) => f.set('q', v)} placeholder="Search name or email" />
        <FilterSelect label="Role" value={f.values.role} onChange={(v) => f.set('role', v)} anyLabel="Any role" options={[...Object.entries(roleLabels).map(([value, text]) => ({ value, text })), { value: 'none', text: 'No role yet' }]} />
        <FilterSelect label="Access" value={f.values.active} onChange={(v) => f.set('active', v)} anyLabel="Active and deactivated" options={[{ value: 'true', text: 'Active' }, { value: 'false', text: 'Deactivated' }]} />
      </FilterBar>
      <DataTable columns={columns} rows={rows} query={query} rowKey={(u) => u.id} meta={meta} paging={paging} filtered={f} empty={{ title: 'No users yet' }} />
      <EditUser user={editing} onClose={() => setEditing(null)} />
    </Card>
  )
}
