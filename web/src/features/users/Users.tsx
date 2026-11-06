import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useEffect, useState, type FormEvent } from 'react'
import { Badge, Button, Card, EmptyState, ErrorState, Field, FormError, Input, Modal, PageHeader, Pagination, SkeletonRows, Td, Th, useConfirmSave } from '../../components/ui'
import { list, listAll, post, put, queryString } from '../../lib/api'
import { formatDateTime } from '../../lib/format'
import { useClampPage, usePaging } from '../../lib/paging'
import { useMe } from '../../lib/session'
import type { Role, User } from '../../lib/types'
import { useChecks, useDebounced } from '../../components/workflow'
import { email as emailIssue, issues as check } from '../../lib/validation'

function RolesDrawer({ user, onClose }: { user: User | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const roles = useQuery({ queryKey: ['roles'], queryFn: () => list<Role>('/roles'), enabled: user !== null })
  const [chosen, setChosen] = useState<Set<string>>(new Set())
  useEffect(() => {
    if (user && roles.data) setChosen(new Set(roles.data.data.filter((r) => user.roles.includes(r.name)).map((r) => r.id)))
  }, [user, roles.data])
  const confirmSave = useConfirmSave()
  const save = useMutation({ meta: { success: 'Roles saved.', inlineError: true },
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
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => confirmSave(`the roles of ${user?.email}`, () => save.mutate())}>Save roles</Button></>}>
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

function ScopeDialog({ user, onClose }: { user: User | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const services = useQuery({ queryKey: ['business-services', 'all'], enabled: user !== null, queryFn: () => listAll<{ id: string; name: string }>('/business-services') })
  const [chosen, setChosen] = useState<Set<string>>(new Set())
  useEffect(() => { setChosen(new Set(user?.businessServiceIds ?? [])) }, [user])
  const confirmSave = useConfirmSave()
  const save = useMutation({ meta: { success: 'Business services saved.', inlineError: true },
    mutationFn: () => put(`/users/${user?.id}/scope`, { businessServiceIds: [...chosen] }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['users'] }); onClose() },
  })
  const toggle = (id: string) => setChosen((prev) => {
    const next = new Set(prev)
    if (!next.delete(id)) next.add(id)
    return next
  })
  return (
    <Modal open={user !== null} onClose={onClose} title="Assign business services" subtitle={user?.email}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => confirmSave(`the business services of ${user?.email}`, () => save.mutate())}>Save</Button></>}>
      <p className="mb-2 text-[13px] text-ink-2">They see subnets and addresses of these services, and scoped reports, without infrastructure details.</p>
      {services.isPending && <SkeletonRows rows={5} />}
      {services.error && <ErrorState error={services.error} onRetry={services.refetch} />}
      {services.data?.length === 0 && <EmptyState title="No business services yet" body="Add them under Network Management first." />}
      <fieldset className="max-h-72 space-y-0.5 overflow-y-auto">
        <legend className="sr-only">Business services</legend>
        {services.data?.map((s) => (
          <label key={s.id} className="flex cursor-pointer items-center gap-2.5 rounded-md p-1.5 hover:bg-surface-2">
            <input type="checkbox" className="size-4 accent-[var(--accent)]" checked={chosen.has(s.id)} onChange={() => toggle(s.id)} />
            <span className="text-ink">{s.name}</span>
          </label>
        ))}
      </fieldset>
      <div className="mt-3"><FormError error={save.error} /></div>
    </Modal>
  )
}

function InviteDrawer({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [email, setEmail] = useState('')
  const invite = useMutation({ meta: { success: 'Invitation sent.', inlineError: true },
    mutationFn: () => post<void>('/users/invite', { email }),
    onSuccess: () => { setEmail(''); onClose() },
  })
  const checks = useChecks(check({ email: emailIssue(email) }), open)
  const submit = (e: FormEvent) => { e.preventDefault(); checks.submit(() => invite.mutate()) }
  return (
    <Modal open={open} onClose={onClose} title="Invite user"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" type="submit" form="invite" busy={invite.isPending}>Send invitation</Button></>}>
      <form id="invite" noValidate onSubmit={submit} className="space-y-4">
        <Field label="Work email" htmlFor="invite-email" error={checks.shown.email} hint="They get an email link, and no access until you assign a role.">
          <Input id="invite-email" type="email" autoFocus value={email} onChange={(e) => setEmail(e.target.value)} />
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
  const [scoping, setScoping] = useState<User | null>(null)
  const [inviting, setInviting] = useState(false)
  const [search, setSearch] = useState('')
  const q = useDebounced(search.trim())
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, q })
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['users', qs], enabled: canManage, placeholderData: keepPreviousData,
    queryFn: () => list<User>(`/users${qs}`),
  })
  useClampPage(paging, data?.meta)
  return (
    <>
      <PageHeader title="User Management" description="New users have no access until they have a role."
        actions={canManage && <Button variant="primary" onClick={() => setInviting(true)}><Plus size={16} aria-hidden /> Invite user</Button>} />
      <Card>
        {!canManage && <EmptyState title="You don't have access to user management" />}
        {canManage && <div className="border-b border-line p-2.5"><Input aria-label="Search users" className="max-w-xs" placeholder="Search by email or name" value={search} onChange={(e) => { setSearch(e.target.value); paging.setPage(1) }} /></div>}
        {canManage && isPending && <SkeletonRows />}
        {error && <ErrorState error={error} onRetry={refetch} />}
        {data?.data.length === 0 && <EmptyState title="No users match" />}
        {data && data.data.length > 0 && (
          <>
            <div className="overflow-x-auto">
              <table className="w-full border-collapse">
                <thead><tr><Th>Email</Th><Th>Roles</Th><Th>Status</Th><Th>Last sign-in</Th><Th /></tr></thead>
                <tbody>
                  {data.data.map((u) => (
                    <tr key={u.id}>
                      <Td className="font-medium text-ink">{u.email}</Td>
                      <Td><div className="flex flex-wrap gap-1">{u.roles.length ? u.roles.map((r) => <Badge key={r} tone="accent">{r}</Badge>) : <span className="text-ink-3">No role</span>}</div>
                        {u.scoped && <span className="mt-0.5 block text-[12px] text-ink-3">{u.businessServices.length ? u.businessServices.join(', ') : 'No business services yet'}</span>}</Td>
                      <Td>{u.isActive ? <Badge tone="ok">Active</Badge> : <Badge tone="danger">Inactive</Badge>}</Td>
                      <Td className="text-ink-2">{u.lastLoginAt ? formatDateTime(u.lastLoginAt) : '—'}</Td>
                      <Td className="text-right whitespace-nowrap">{u.id !== me.id && <>{u.scoped && <Button variant="ghost" className="h-7 px-2" onClick={() => setScoping(u)}>Business services</Button>}<Button variant="ghost" className="h-7 px-2" onClick={() => setEditing(u)}>Roles</Button></>}</Td>
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
      <ScopeDialog user={scoping} onClose={() => setScoping(null)} />
      <InviteDrawer open={inviting} onClose={() => setInviting(false)} />
    </>
  )
}
