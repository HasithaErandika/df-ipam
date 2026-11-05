import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'
import { Badge, Button, Card, Modal, EmptyState, ErrorState, Field, FormError, Input, PageHeader, Pagination, SkeletonRows, Td, Th } from '../components/ui'
import { list, post, put } from '../lib/api'
import { formatDateTime } from '../lib/format'
import { useClampPage, usePaging } from '../lib/paging'
import { useMe } from '../lib/session'
import type { Role, User } from '../lib/types'

function RolesDrawer({ user, onClose }: { user: User | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const roles = useQuery({ queryKey: ['roles'], queryFn: () => list<Role>('/roles'), enabled: user !== null })
  const [chosen, setChosen] = useState<Set<string>>(new Set())
  useEffect(() => {
    if (user && roles.data) setChosen(new Set(roles.data.data.filter((r) => user.roles.includes(r.name)).map((r) => r.id)))
  }, [user, roles.data])
  const save = useMutation({
    mutationFn: () => put(`/users/${user?.id}/roles`, { roleIds: [...chosen] }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['users'] }); onClose() },
  })
  const toggle = (id: string) => setChosen((prev) => {
    const next = new Set(prev)
    if (!next.delete(id)) next.add(id)
    return next
  })
  return (
    <Modal open={user !== null} onClose={onClose} title="Assign roles" subtitle={user?.email}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => save.mutate()}>Save roles</Button></>}>
      {roles.isPending && <SkeletonRows rows={6} />}
      {roles.error && <ErrorState error={roles.error} onRetry={roles.refetch} />}
      <fieldset className="space-y-1">
        <legend className="sr-only">Roles</legend>
        {roles.data?.data.map((r) => (
          <label key={r.id} className="flex cursor-pointer gap-2.5 rounded-md p-2 hover:bg-surface-2">
            <input type="checkbox" className="mt-0.5 size-4 accent-[var(--accent)]" checked={chosen.has(r.id)} onChange={() => toggle(r.id)} />
            <span><span className="block font-medium text-ink">{r.name}</span><span className="block text-[13px] text-ink-2">{r.description}</span></span>
          </label>
        ))}
      </fieldset>
      <div className="mt-3"><FormError error={save.error} /></div>
    </Modal>
  )
}

function InviteDrawer({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [email, setEmail] = useState('')
  const invite = useMutation({
    mutationFn: () => post<void>('/users/invite', { email }),
    onSuccess: () => { setEmail(''); onClose() },
  })
  const submit = (e: FormEvent) => { e.preventDefault(); invite.mutate() }
  return (
    <Modal open={open} onClose={onClose} title="Invite user"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" type="submit" form="invite" busy={invite.isPending}>Send invitation</Button></>}>
      <form id="invite" onSubmit={submit} className="space-y-4">
        <Field label="Work email" htmlFor="invite-email" hint="They get an email link, and no access until you assign a role.">
          <Input id="invite-email" type="email" required autoFocus value={email} onChange={(e) => setEmail(e.target.value)} />
        </Field>
        <FormError error={invite.error} />
      </form>
    </Modal>
  )
}

export function Users() {
  const me = useMe()
  const canManage = me.permissions.includes('users:manage')
  const paging = usePaging()
  const [editing, setEditing] = useState<User | null>(null)
  const [inviting, setInviting] = useState(false)
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['users', paging.page, paging.pageSize], enabled: canManage, placeholderData: keepPreviousData,
    queryFn: () => list<User>(`/users?page=${paging.page}&pageSize=${paging.pageSize}`),
  })
  useClampPage(paging, data?.meta)
  return (
    <>
      <PageHeader title="User Management" description="New users have no access until they have a role."
        actions={canManage && <Button variant="primary" onClick={() => setInviting(true)}><Plus size={16} aria-hidden /> Invite user</Button>} />
      <Card>
        {!canManage && <EmptyState title="You don't have access to user management" />}
        {canManage && isPending && <SkeletonRows />}
        {error && <ErrorState error={error} onRetry={refetch} />}
        {data && (
          <>
            <div className="overflow-x-auto">
              <table className="w-full border-collapse">
                <thead><tr><Th>Email</Th><Th>Roles</Th><Th>Status</Th><Th>Last sign-in</Th><Th /></tr></thead>
                <tbody>
                  {data.data.map((u) => (
                    <tr key={u.id}>
                      <Td className="font-medium text-ink">{u.email}</Td>
                      <Td><div className="flex flex-wrap gap-1">{u.roles.length ? u.roles.map((r) => <Badge key={r} tone="accent">{r}</Badge>) : <span className="text-ink-3">No role</span>}</div></Td>
                      <Td>{u.isActive ? <Badge tone="ok">Active</Badge> : <Badge tone="danger">Inactive</Badge>}</Td>
                      <Td className="text-ink-2">{u.lastLoginAt ? formatDateTime(u.lastLoginAt) : '—'}</Td>
                      <Td className="text-right">{u.id !== me.id && <Button variant="ghost" className="h-7 px-2" onClick={() => setEditing(u)}>Roles</Button>}</Td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
          </>
        )}
      </Card>
      <RolesDrawer user={editing} onClose={() => setEditing(null)} />
      <InviteDrawer open={inviting} onClose={() => setInviting(false)} />
    </>
  )
}
