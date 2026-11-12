import { useMutation } from '@tanstack/react-query'
import { Check, Lock, Pencil, Plus, Trash2, Users } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Badge, Button, Card, EmptyState, ErrorState, Field, FormError, Input, Modal, Notice, RowMenu, Select, SkeletonRows, Textarea, useConfirm, useConfirmSave } from '../../components/ui'
import { cx } from '../../components/ui/cx'
import { del, post, put } from '../../lib/api'
import { listChanges } from '../../lib/changes'
import { permissionGroups, permissionLabel } from '../../lib/permissions'
import { useInvalidate, useRoles } from '../../lib/queries'
import type { RoleDef } from '../../lib/types'

function PermissionPicker({ value, onChange, locked }: { value: string[]; onChange: (v: string[]) => void; locked?: boolean }) {
  const toggle = (k: string) => onChange(value.includes(k) ? value.filter((x) => x !== k) : [...value, k])
  return (
    <div className="space-y-4">
      {permissionGroups.map(({ group, items }) => (
        <fieldset key={group}>
          <legend className="mb-1.5 text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase">{group}</legend>
          <div className="grid gap-1.5 sm:grid-cols-2">
            {items.map((p) => {
              const on = value.includes(p.key)
              return (
                <button key={p.key} type="button" role="checkbox" aria-checked={on} disabled={locked} onClick={() => toggle(p.key)}
                  className={cx('flex items-start gap-2.5 rounded-md border px-3 py-2 text-left transition-colors disabled:cursor-not-allowed',
                    on ? 'border-accent bg-accent-soft/50' : 'border-line bg-surface hover:border-ink-3', !locked && 'cursor-pointer')}>
                  <span className={cx('mt-0.5 grid size-4 shrink-0 place-items-center rounded-[3px] border', on ? 'border-accent bg-accent text-white' : 'border-line-strong bg-surface')}>
                    {on && <Check size={12} strokeWidth={3} aria-hidden />}
                  </span>
                  <span className="min-w-0">
                    <span className="block text-[13px] font-medium text-ink">{p.label}</span>
                    <span className="block text-[12px] leading-snug text-ink-3">{p.detail}</span>
                  </span>
                </button>
              )
            })}
          </div>
        </fieldset>
      ))}
    </div>
  )
}

function RoleForm({ open, role, roles, onClose }: { open: boolean; role?: RoleDef; roles: RoleDef[]; onClose: () => void }) {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [perms, setPerms] = useState<string[]>([])
  const refresh = useInvalidate('/roles', '/users', 'me')
  const confirmSave = useConfirmSave()
  useEffect(() => { if (open) { setName(role?.name ?? ''); setDescription(role?.description ?? ''); setPerms(role?.permissions ?? ['inventory:view']) } }, [open, role])
  const save = useMutation({
    mutationFn: () => (role ? put(`/roles/${role.key}`, { name, description, permissions: perms }) : post('/roles', { name, description, permissions: perms })),
    meta: { inlineError: true, success: `${name.trim()} saved.` }, onSuccess: () => { void refresh(); onClose() },
  })
  const locked = role?.fullAccess
  const sorted = (p: string[]) => [...p].sort()
  const changes = role ? [
    ...listChanges({ name: role.name, description: role.description }, { name: name.trim(), description: description.trim() }, { name: 'Name', description: 'Description' }),
    ...perms.filter((p) => !role.permissions.includes(p)).map((p) => `Grants: ${permissionLabel(p)}`),
    ...role.permissions.filter((p) => !perms.includes(p)).map((p) => `No longer grants: ${permissionLabel(p)}`),
  ] : []
  const nameIssue = !name.trim() ? 'Name is required.' : undefined
  const unchanged = Boolean(role) && changes.length === 0
  return (
    <Modal open={open} onClose={onClose} size="lg" title={role ? `Edit ${role.name}` : 'Add a role'}
      subtitle={role ? `${role.users} ${role.users === 1 ? 'person holds' : 'people hold'} this role. Changes apply on their next request.` : 'A name for the job, and what it may do.'}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} disabled={Boolean(nameIssue) || unchanged}
        onClick={() => (role ? void confirmSave(role.name, () => save.mutate(), undefined, changes) : save.mutate())}>{role ? 'Save changes' : 'Add role'}</Button></>}>
      <div className="space-y-4">
        <div className="grid gap-3 sm:grid-cols-[1fr_220px]">
          <Field label="Name" htmlFor="r-name" error={name ? nameIssue : undefined}><Input id="r-name" value={name} maxLength={60} placeholder="Network engineer" onChange={(e) => setName(e.target.value)} /></Field>
          {!role && (
            <Field label="Copy permissions from" htmlFor="r-copy">
              <Select id="r-copy" value="" onChange={(e) => { const from = roles.find((r) => r.key === e.target.value); if (from) setPerms(sorted(from.permissions)) }}>
                <option value="">Choose a role…</option>{roles.map((r) => <option key={r.key} value={r.key}>{r.name}</option>)}
              </Select>
            </Field>
          )}
        </div>
        <Field label="What it is for" htmlFor="r-desc"><Textarea id="r-desc" rows={2} maxLength={300} value={description} onChange={(e) => setDescription(e.target.value)} /></Field>
        {locked && <Notice>This role holds every permission, including any added later. Its permissions can't be changed.</Notice>}
        <PermissionPicker value={perms} onChange={setPerms} locked={locked} />
        {!locked && !perms.includes('inventory:view') && <Notice tone="warn">Without "See the network", people with this role see almost nothing.</Notice>}
        <FormError error={save.error} />
      </div>
    </Modal>
  )
}

function RoleCard({ role, onEdit, onDelete }: { role: RoleDef; onEdit: () => void; onDelete: () => void }) {
  return (
    <article className="flex flex-col rounded-lg border border-line bg-surface p-4">
      <header className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <h3 className="flex items-center gap-2 text-[14px] font-semibold text-ink">{role.name}{role.isSystem && <Lock size={13} className="text-ink-3" aria-label="System role" />}</h3>
          <p className="mt-0.5 text-[12.5px] text-ink-3">{role.description || 'No description.'}</p>
        </div>
        <RowMenu actions={[
          { label: 'Edit', icon: <Pencil size={14} aria-hidden />, onSelect: onEdit },
          { label: 'Delete', icon: <Trash2 size={14} aria-hidden />, danger: true, hidden: role.isSystem || role.users > 0, onSelect: onDelete },
        ]} />
      </header>
      <div className="mt-3 flex flex-wrap gap-1">
        {role.fullAccess ? <Badge tone="accent">Every permission</Badge>
          : role.permissions.length === 0 ? <span className="text-[12.5px] text-ink-3">No permissions</span>
            : role.permissions.map((p) => <Badge key={p}>{permissionLabel(p)}</Badge>)}
      </div>
      <footer className="mt-auto flex items-center gap-1.5 pt-3 text-[12px] text-ink-3"><Users size={13} aria-hidden />{role.users} {role.users === 1 ? 'person' : 'people'}</footer>
    </article>
  )
}

export function RolesTab() {
  const q = useRoles()
  const [form, setForm] = useState<{ role?: RoleDef } | null>(null)
  const confirm = useConfirm()
  const refresh = useInvalidate('/roles')
  const remove = useMutation({ mutationFn: (r: RoleDef) => del(`/roles/${r.key}`), meta: { success: (r: RoleDef) => `${r.name} deleted.` }, onSuccess: () => void refresh() })
  return (
    <Card title="Roles" subtitle="Each role is a job, such as network engineer or IT officer, and the permissions it grants. A person holds one role."
      action={<Button variant="primary" onClick={() => setForm({})}><Plus size={14} aria-hidden />Add role</Button>}>
      {q.isPending ? <SkeletonRows rows={4} />
        : q.isError ? <ErrorState error={q.error} onRetry={() => void q.refetch()} />
          : q.data.length === 0 ? <EmptyState title="No roles" />
            : (
              <div className="grid gap-3 p-4 md:grid-cols-2 xl:grid-cols-3">
                {q.data.map((r) => (
                  <RoleCard key={r.key} role={r} onEdit={() => setForm({ role: r })} onDelete={async () => {
                    if (await confirm({ title: `Delete ${r.name}?`, body: 'Nobody holds it, so nothing else changes.', confirmLabel: 'Delete role' })) remove.mutate(r)
                  }} />
                ))}
              </div>
            )}
      <RoleForm open={form !== null} role={form?.role} roles={q.data ?? []} onClose={() => setForm(null)} />
    </Card>
  )
}
