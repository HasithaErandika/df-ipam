import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Badge, Button, Card, ConfirmDialog, EmptyState, ErrorState, Field, FormError, Input, Modal, Mono, MoreOptions, PageHeader, SkeletonRows, Td, Th, useConfirmSave } from '../../components/ui'
import { del, get, post, put } from '../../lib/api'
import { useCan, useMe } from '../../lib/session'
import type { MatrixRole, PermissionMatrix } from '../../lib/types'
import { useChecks } from '../../components/workflow'
import { issues as check, maxLength, required } from '../../lib/validation'

function RoleEditor({ role, permissions, onClose }: { role: MatrixRole | null; permissions: string[]; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [chosen, setChosen] = useState<Set<string>>(new Set())
  const [search, setSearch] = useState('')
  useEffect(() => { setChosen(new Set(role?.permissions)); setSearch('') }, [role])
  const confirmSave = useConfirmSave()
  const save = useMutation({ meta: { success: 'Permissions saved.', inlineError: true },
    mutationFn: () => put(`/roles/${role?.id}/permissions`, { permissions: [...chosen] }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['matrix'] }); onClose() },
  })
  if (!role) return null
  const toggle = (p: string) => setChosen((prev) => {
    const next = new Set(prev)
    if (!next.delete(p)) next.add(p)
    return next
  })
  return (
    <Modal open onClose={onClose} title={`Permissions: ${role.name}`} subtitle={role.description} size="lg"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => confirmSave(`the permissions of ${role?.name}`, () => save.mutate())}>Save</Button></>}>
      <Input aria-label="Search permissions" className="mb-2" placeholder="Search permissions" value={search} onChange={(e) => setSearch(e.target.value)} />
      <fieldset className="grid gap-1 sm:grid-cols-2">
        <legend className="sr-only">Permissions</legend>
        {permissions.filter((p) => p.includes(search.trim().toLowerCase())).map((p) => (
          <label key={p} className="flex cursor-pointer items-center gap-2 rounded p-1.5 hover:bg-surface-2">
            <input type="checkbox" className="size-4 accent-[var(--accent)]" checked={chosen.has(p)} onChange={() => toggle(p)} /><Mono>{p}</Mono>
          </label>
        ))}
      </fieldset>
      <div className="mt-3"><FormError error={save.error} /></div>
      <p className="mt-3 text-[12px] text-ink-3">{role.userCount} {role.userCount === 1 ? 'person holds' : 'people hold'} this role. The change applies on their next request.</p>
    </Modal>
  )
}

function RoleDetails({ role, onClose }: { role: MatrixRole | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  useEffect(() => { setName(role?.name ?? ''); setDescription(role?.description ?? '') }, [role])
  const confirmSave = useConfirmSave()
  const save = useMutation({ meta: { success: 'Role saved.', inlineError: true },
    mutationFn: () => put(`/roles/${role?.id}`, { name, description }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['matrix'] }); queryClient.invalidateQueries({ queryKey: ['roles'] }); onClose() },
  })
  const checks = useChecks(check({ name: required(name, 'Name') ?? maxLength(name, 100, 'Name') }))
  if (!role) return null
  return (
    <Modal open onClose={onClose} title={`Edit ${role.name}`}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} onClick={() => checks.submit(() => confirmSave(`role ${role?.name}`, () => save.mutate()))}>Save</Button></>}>
      <div className="space-y-3">
        <Field label="Name" htmlFor="edit-role-name" error={checks.shown.name} hint={role.isSystem ? 'Default roles keep their names.' : undefined}>
          <Input id="edit-role-name" value={name} disabled={role.isSystem} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label="Description" htmlFor="edit-role-description"><Input id="edit-role-description" value={description} onChange={(e) => setDescription(e.target.value)} /></Field>
      </div>
      <div className="mt-3"><FormError error={save.error} /></div>
    </Modal>
  )
}

function NewRole({ open, onClose }: { open: boolean; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const create = useMutation({ meta: { success: 'Role created.', inlineError: true },
    mutationFn: () => post('/roles', { name, description }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['matrix'] }); setName(''); setDescription(''); onClose() },
  })
  const checks = useChecks(check({ name: required(name, 'Name') ?? maxLength(name, 100, 'Name') }), open)
  return (
    <Modal open={open} onClose={onClose} title="New role"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={create.isPending} onClick={() => checks.submit(() => create.mutate())}>Create</Button></>}>
      <div className="space-y-3">
        <Field label="Name" htmlFor="role-name" error={checks.shown.name}><Input id="role-name" value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="Description" htmlFor="role-description"><Input id="role-description" value={description} onChange={(e) => setDescription(e.target.value)} /></Field>
      </div>
      <div className="mt-3"><FormError error={create.error} /></div>
    </Modal>
  )
}

export function Roles() {
  const queryClient = useQueryClient()
  const me = useMe()
  const canManage = useCan('users:manage')
  const canAudit = useCan('audit:view')
  const canView = canManage || canAudit
  const [editing, setEditing] = useState<MatrixRole | null>(null)
  const [details, setDetails] = useState<MatrixRole | null>(null)
  const [adding, setAdding] = useState(false)
  const [removing, setRemoving] = useState<MatrixRole | null>(null)
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['matrix'], enabled: canView, queryFn: () => get<PermissionMatrix>('/admin/permission-matrix') })
  const remove = useMutation({ meta: { success: 'Role deleted.', inlineError: true },
    mutationFn: () => del(`/roles/${removing?.id}`),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['matrix'] }); setRemoving(null) },
  })
  if (!canView) return <><PageHeader title="Roles and Permissions" /><Card><EmptyState title="You don't have access to roles" /></Card></>
  return (
    <>
      <PageHeader title="Roles and Permissions" description="What each role can do. Separation of duties always applies: nobody approves their own request."
        actions={canManage && <Button variant="primary" onClick={() => setAdding(true)}><Plus size={14} aria-hidden /> New role</Button>} />
      {data && (
        <Card title="Roles" className="mb-3">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse">
              <thead><tr><Th>Role</Th><Th>Description</Th><Th className="text-right">People</Th><Th className="text-right">Permissions</Th>{canManage && <Th className="text-right">Actions</Th>}</tr></thead>
              <tbody>
                {data.roles.map((r) => {
                  const held = me.roles.includes(r.name)
                  return (
                    <tr key={r.id} className="hover:bg-surface-2">
                      <Td className="font-medium whitespace-nowrap text-ink">{r.name}{r.isCustomised && r.isSystem && <span className="ml-2"><Badge tone="accent">Edited</Badge></span>}</Td>
                      <Td className="text-ink-2">{r.description}</Td>
                      <Td className="text-right text-ink-2 tabular-nums">{r.userCount}</Td>
                      <Td className="text-right text-ink-2 tabular-nums">{r.permissions.length}</Td>
                      {canManage && (
                        <Td className="text-right whitespace-nowrap">
                          <span className="inline-flex items-center gap-1">
                            {r.locked && <span className="text-xs text-ink-3">Always holds every permission</span>}
                            {!r.locked && held && <span className="text-xs text-ink-3">You hold this role, so another administrator must change it</span>}
                            {!r.locked && !held && <Button onClick={() => setEditing(r)}>Edit permissions</Button>}
                            {!r.locked && <Button onClick={() => setDetails(r)}>Edit details</Button>}
                            {!r.isSystem && <Button variant="ghost" onClick={() => setRemoving(r)}>Delete</Button>}
                          </span>
                        </Td>
                      )}
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </Card>
      )}
      {data && <MoreOptions label="Compare all permissions">
      <Card title="Permission matrix">
        {isPending && <SkeletonRows rows={8} />}
        {error && <ErrorState error={error} onRetry={refetch} />}
        {data && (
          <div className="overflow-x-auto">
            <table className="w-full border-collapse">
              <thead>
                <tr>
                  <Th>Permission</Th>
                  {data.roles.map((r) => <Th key={r.id} className="min-w-28 align-bottom whitespace-normal">{r.name}</Th>)}
                </tr>
              </thead>
              <tbody>
                {data.permissions.map((p) => (
                  <tr key={p} className="hover:bg-surface-2">
                    <Td><Mono>{p}</Mono></Td>
                    {data.roles.map((r) => <Td key={r.id} className="text-ink-2">{r.permissions.includes(p) ? <span aria-label="Allowed">✓</span> : <span className="sr-only">Not allowed</span>}</Td>)}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
      </MoreOptions>}
      <RoleEditor role={editing} permissions={data?.permissions ?? []} onClose={() => setEditing(null)} />
      <RoleDetails role={details} onClose={() => setDetails(null)} />
      <NewRole open={adding} onClose={() => setAdding(false)} />
      <ConfirmDialog open={removing !== null} title="Delete this role?" body="A role can be deleted only when nobody holds it and no approval workflow uses it."
        confirmLabel="Delete" busy={remove.isPending} error={remove.error} onConfirm={() => remove.mutate()} onCancel={() => setRemoving(null)} />
    </>
  )
}
