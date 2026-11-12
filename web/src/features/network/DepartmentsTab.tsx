import { useMutation } from '@tanstack/react-query'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Button, Card, DataTable, Field, FormError, Input, Modal, Notice, RowMenu, Textarea, useConfirm, useConfirmSave, type Column } from '../../components/ui'
import { del, post, put } from '../../lib/api'
import { useDepartments, useInvalidate } from '../../lib/queries'
import { listChanges } from '../../lib/changes'
import type { Department } from '../../lib/types'

const uses = (d: Department) => d.devices + d.addresses

function DepartmentForm({ open, dept, onClose }: { open: boolean; dept?: Department; onClose: () => void }) {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const refresh = useInvalidate('/departments', '/devices', '/ip-addresses', '/discovery')
  const confirmSave = useConfirmSave()
  useEffect(() => { if (open) { setName(dept?.name ?? ''); setDescription(dept?.description ?? '') } }, [open, dept])
  const save = useMutation({
    mutationFn: () => (dept ? put(`/departments/${dept.id}`, { name, description }) : post('/departments', { name, description })),
    meta: { inlineError: true, success: `${name.trim()} saved.` }, onSuccess: () => { void refresh(); onClose() },
  })
  const nameIssue = !name.trim() ? 'Name is required.' : name.length > 120 ? 'Keep the name under 120 characters.' : undefined
  const changes = dept ? listChanges({ name: dept.name, description: dept.description }, { name: name.trim(), description: description.trim() }, { name: 'Name', description: 'Description' }) : []
  const renamed = dept && name.trim() !== dept.name && uses(dept) > 0
  return (
    <Modal open={open} onClose={onClose} title={dept ? `Edit ${dept.name}` : 'Add a department'} subtitle="People pick it when they approve a device or register an address."
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" disabled={Boolean(nameIssue) || (Boolean(dept) && changes.length === 0)} busy={save.isPending}
        onClick={() => (dept ? void confirmSave(dept.name, () => save.mutate(), undefined, changes) : save.mutate())}>{dept ? 'Save changes' : 'Add department'}</Button></>}>
      <div className="space-y-4">
        <Field label="Name" htmlFor="dp-name" error={name ? nameIssue : undefined}><Input id="dp-name" value={name} maxLength={120} placeholder="Finance" onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="Description (optional)" htmlFor="dp-desc"><Textarea id="dp-desc" rows={2} maxLength={300} value={description} onChange={(e) => setDescription(e.target.value)} /></Field>
        {renamed && <Notice tone="warn">The {dept.devices} device{dept.devices === 1 ? '' : 's'} and {dept.addresses} register entr{dept.addresses === 1 ? 'y' : 'ies'} in {dept.name} move to the new name.</Notice>}
        <FormError error={save.error} />
      </div>
    </Modal>
  )
}

export function DepartmentsTab() {
  const q = useDepartments()
  const [form, setForm] = useState<{ dept?: Department } | null>(null)
  const confirm = useConfirm()
  const refresh = useInvalidate('/departments')
  const remove = useMutation({ mutationFn: (d: Department) => del(`/departments/${d.id}`), meta: { success: (d: Department) => `${d.name} deleted.` }, onSuccess: () => void refresh() })
  const columns: Column<Department>[] = [
    { key: 'name', header: 'Department', cell: (d) => <div className="min-w-0"><div className="font-medium text-ink">{d.name}</div>{d.description && <div className="truncate text-[12px] text-ink-3">{d.description}</div>}</div> },
    { key: 'devices', header: 'Devices', className: 'text-right tabular-nums', cell: (d) => d.devices },
    { key: 'addresses', header: 'Register entries', className: 'text-right tabular-nums', cell: (d) => d.addresses },
    { key: 'act', header: '', className: 'w-10 text-right', cell: (d) => (
      <RowMenu actions={[
        { label: 'Edit', icon: <Pencil size={14} aria-hidden />, onSelect: () => setForm({ dept: d }) },
        { label: 'Delete', icon: <Trash2 size={14} aria-hidden />, danger: true, hidden: uses(d) > 0, onSelect: async () => {
          if (await confirm({ title: `Delete ${d.name}?`, body: 'It is no longer offered when approving devices or registering addresses.', confirmLabel: 'Delete department' })) remove.mutate(d)
        } },
      ]} />
    ) },
  ]
  return (
    <Card title="Departments" subtitle="The list people pick from for devices and register entries. A department in use can be renamed, not deleted."
      action={<Button variant="primary" onClick={() => setForm({})}><Plus size={14} aria-hidden />Add department</Button>}>
      <DataTable columns={columns} rows={q.data ?? []} query={q} rowKey={(d) => d.id}
        empty={{ title: 'No departments yet', body: 'Add the departments that own devices and addresses, such as Finance or IT. Until then, people type the department by hand.' }} />
      <DepartmentForm open={form !== null} dept={form?.dept} onClose={() => setForm(null)} />
    </Card>
  )
}
