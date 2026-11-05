import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Search, Trash2 } from 'lucide-react'
import { useState, type FormEvent, type ReactNode } from 'react'
import { Link } from 'react-router'
import {
  Badge, Button, Card, ConfirmDialog, EmptyState, ErrorState, Field, FormError, Input, Modal, Mono, Pagination, Select, SkeletonRows, Td, Th,
} from '../../components/ui'
import { ApiError, del, list, patch, post, queryString } from '../../lib/api'
import { environments, formatDateTime } from '../../lib/format'
import { optionLabel, useDebounced, useOptions } from '../../lib/hooks'
import { useClampPage, usePaging } from '../../lib/paging'
import { useCan } from '../../lib/session'

export type Row = Record<string, unknown> & { id: string }

export interface FieldDef {
  key: string
  label: string
  type: 'text' | 'mono' | 'number' | 'checkbox' | 'choice' | 'record' | 'date'
  required?: boolean
  /** Fixed choices for a text column, value to label. */
  choices?: Record<string, string>
  /** Where a related record's options come from. */
  path?: string
  hint?: string
  /** Set on create only: changing it later would orphan dependent records. */
  createOnly?: boolean
}

export interface ColumnDef {
  label: string
  render: (row: Row) => ReactNode
  className?: string
}

export interface CatalogDef {
  tab: string
  label: string
  singular: string
  path: string
  description: string
  empty: string
  fields: FieldDef[]
  columns: ColumnDef[]
  /** Overrides inventory:create, edit and delete when the entity needs another permission. */
  writePermission?: string
  searchLabel?: string
  actions?: (row: Row, open: (node: ReactNode) => void) => ReactNode
}

const text = (v: unknown) => (v === null || v === undefined || v === '' ? '—' : String(v))

function initial(fields: FieldDef[], row: Row | null): Record<string, string | boolean> {
  const out: Record<string, string | boolean> = {}
  for (const f of fields) {
    const v = row?.[f.key]
    if (f.type === 'checkbox') out[f.key] = Boolean(v)
    else if (f.type === 'date') out[f.key] = typeof v === 'string' ? v.slice(0, 10) : ''
    else if (f.type === 'choice') out[f.key] = v === undefined || v === null ? Object.keys(f.choices ?? {})[0] ?? '' : String(v)
    else out[f.key] = v === undefined || v === null ? '' : String(v)
  }
  return out
}

/** Builds the request body. A new record leaves out empty values; an edit sends them, which clears a link. */
function body(fields: FieldDef[], values: Record<string, string | boolean>, editing: boolean) {
  const out: Record<string, unknown> = {}
  for (const f of fields) {
    if (editing && f.createOnly) continue
    const v = values[f.key]
    if (f.type === 'checkbox') out[f.key] = v
    else if (f.type === 'number') { if (v !== '') out[f.key] = Number(v) }
    else if (v !== '' || editing) out[f.key] = v
  }
  return out
}

function FieldInput({ f, value, onChange, error, disabled }:
  { f: FieldDef; value: string | boolean; onChange: (v: string | boolean) => void; error?: string; disabled?: boolean }) {
  const options = useOptions(f.path ?? '', f.type === 'record')
  const id = `field-${f.key}`
  if (f.type === 'checkbox') {
    return (
      <label className="flex items-center gap-2 pt-5 text-ink">
        <input id={id} type="checkbox" className="size-4 accent-[var(--accent)]" checked={Boolean(value)} onChange={(e) => onChange(e.target.checked)} />
        {f.label}
      </label>
    )
  }
  const label = f.required ? f.label : `${f.label} (optional)`
  return (
    <Field label={label} htmlFor={id} error={error} hint={f.hint}>
      {f.type === 'record' ? (
        <Select id={id} value={String(value)} disabled={disabled} onChange={(e) => onChange(e.target.value)}>
          <option value="">{f.required ? 'Select' : 'None'}</option>
          {options.data?.map((o) => <option key={o.id} value={o.id}>{optionLabel(o)}</option>)}
        </Select>
      ) : f.type === 'choice' ? (
        <Select id={id} value={String(value)} onChange={(e) => onChange(e.target.value)}>
          {!f.required && <option value="">None</option>}
          {Object.entries(f.choices ?? {}).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
        </Select>
      ) : (
        <Input id={id} value={String(value)} disabled={disabled} type={f.type === 'number' ? 'number' : f.type === 'date' ? 'date' : 'text'}
          className={f.type === 'mono' ? 'font-mono' : undefined} spellCheck={f.type !== 'mono'} onChange={(e) => onChange(e.target.value)} />
      )}
    </Field>
  )
}

function EditModal({ def, row, open, onClose }: { def: CatalogDef; row: Row | null; open: boolean; onClose: () => void }) {
  const qc = useQueryClient()
  const editing = row !== null
  const [values, setValues] = useState(() => initial(def.fields, row))
  const save = useMutation({
    mutationFn: () => editing ? patch<Row>(`${def.path}/${row.id}`, body(def.fields, values, true)) : post<Row>(def.path, body(def.fields, values, false)),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['catalog', def.path] })
      qc.invalidateQueries({ queryKey: ['options'] })
      qc.invalidateQueries({ queryKey: ['subnets'] })
      onClose()
    },
  })
  const issue = (key: string) => save.error instanceof ApiError ? save.error.details.find((d) => d.field === key)?.issue : undefined
  const submit = (e: FormEvent) => { e.preventDefault(); save.mutate() }
  const formId = `catalog-${def.tab}`
  return (
    <Modal open={open} onClose={onClose} size="lg" title={editing ? `Edit ${def.singular}` : `New ${def.singular}`}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" type="submit" form={formId} busy={save.isPending}>{editing ? 'Save changes' : `Create ${def.singular}`}</Button></>}>
      <form id={formId} noValidate onSubmit={submit} className="grid gap-3.5 sm:grid-cols-2">
        {def.fields.map((f) => (
          <FieldInput key={f.key} f={f} value={values[f.key]} error={issue(f.key)} disabled={editing && f.createOnly}
            onChange={(v) => setValues((prev) => ({ ...prev, [f.key]: v }))} />
        ))}
        <div className="sm:col-span-2">{!(save.error instanceof ApiError && save.error.details.length > 0) && <FormError error={save.error} />}</div>
      </form>
    </Modal>
  )
}

/** A searchable, paged table of one kind of reference record, with create, edit and delete. */
export function CatalogTab({ def }: { def: CatalogDef }) {
  const qc = useQueryClient()
  const canCreate = useCan(def.writePermission ?? 'inventory:create')
  const canEdit = useCan(def.writePermission ?? 'inventory:edit')
  const canDelete = useCan(def.writePermission ?? 'inventory:delete')
  const paging = usePaging(def.tab, 50)
  const [search, setSearch] = useState('')
  const q = useDebounced(search)
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, q })
  const rows = useQuery({ queryKey: ['catalog', def.path, qs], placeholderData: keepPreviousData, queryFn: () => list<Row>(`${def.path}${qs}`) })
  useClampPage(paging, rows.data?.meta)
  const [editing, setEditing] = useState<Row | 'new' | null>(null)
  const [deleting, setDeleting] = useState<Row | null>(null)
  const [extra, setExtra] = useState<ReactNode>(null)
  const remove = useMutation({
    mutationFn: (id: string) => del(`${def.path}/${id}`),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ['catalog', def.path] }); qc.invalidateQueries({ queryKey: ['options'] }); setDeleting(null) },
  })
  const hasRows = (rows.data?.data.length ?? 0) > 0
  return (
    <Card>
      <div className="flex flex-wrap items-center gap-2 border-b border-line p-3">
        <div className="relative min-w-52 flex-1">
          <Search size={15} className="absolute top-1/2 left-2.5 -translate-y-1/2 text-ink-3" aria-hidden />
          <Input aria-label={def.searchLabel ?? 'Filter by name'} placeholder={def.searchLabel ?? 'Filter by name'} className="pl-8" value={search} onChange={(e) => { setSearch(e.target.value); paging.setPage(1) }} />
        </div>
        {canCreate && <Button variant="primary" onClick={() => setEditing('new')}><Plus size={15} aria-hidden /> New {def.singular}</Button>}
      </div>
      <p className="border-b border-line px-3 py-2 text-[13px] text-ink-2">{def.description}</p>
      {rows.isPending && <SkeletonRows rows={5} />}
      {rows.error && <ErrorState error={rows.error} onRetry={rows.refetch} />}
      {rows.data && !hasRows && (q ? <EmptyState title={`No ${def.label.toLowerCase()} match “${q}”`} /> : <EmptyState title={`No ${def.label.toLowerCase()} yet`} body={def.empty} />)}
      {rows.data && hasRows && (
        <>
          <div className="overflow-x-auto">
            <table className="w-full border-collapse">
              <thead><tr>{def.columns.map((c) => <Th key={c.label} className={c.className}>{c.label}</Th>)}<Th className="w-px"><span className="sr-only">Actions</span></Th></tr></thead>
              <tbody>
                {rows.data.data.map((row) => (
                  <tr key={row.id} className="hover:bg-surface-2">
                    {def.columns.map((c) => <Td key={c.label} className={c.className}>{c.render(row)}</Td>)}
                    <Td className="whitespace-nowrap text-right">
                      {def.actions?.(row, setExtra)}
                      {canEdit && <Button variant="ghost" className="h-7 px-1.5" aria-label={`Edit ${String(row.name ?? row.reason ?? row.id)}`} onClick={() => setEditing(row)}><Pencil size={14} /></Button>}
                      {canDelete && <Button variant="ghost" className="h-7 px-1.5 hover:text-danger" aria-label={`Delete ${String(row.name ?? row.reason ?? row.id)}`} onClick={() => { remove.reset(); setDeleting(row) }}><Trash2 size={14} /></Button>}
                    </Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination meta={rows.data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
        </>
      )}
      {editing !== null && <EditModal key={editing === 'new' ? 'new' : editing.id} def={def} row={editing === 'new' ? null : editing} open onClose={() => setEditing(null)} />}
      <ConfirmDialog open={deleting !== null} title={`Delete this ${def.singular}?`}
        body="This can't be undone. Records that still use it block the delete." confirmLabel="Delete"
        busy={remove.isPending} error={remove.error} onConfirm={() => deleting && remove.mutate(deleting.id)} onCancel={() => setDeleting(null)} />
      {extra}
    </Card>
  )
}

// Shared cell renderers.
export const cell = {
  text: (key: string) => (r: Row) => <span className="text-ink-2">{text(r[key])}</span>,
  name: (key = 'name') => (r: Row) => <span className="font-medium text-ink">{text(r[key])}</span>,
  mono: (key: string) => (r: Row) => r[key] ? <Mono>{String(r[key])}</Mono> : <span className="text-ink-3">—</span>,
  choice: (key: string, labels: Record<string, string>) => (r: Row) => <span className="text-ink-2">{labels[String(r[key])] ?? text(r[key])}</span>,
  date: (key: string) => (r: Row) => <span className="text-ink-2">{r[key] ? formatDateTime(String(r[key])) : '—'}</span>,
  link: (label: string, to: (r: Row) => string) => (r: Row) => <Link to={to(r)} className="text-[13px] text-accent hover:underline">{label}</Link>,
  badge: (key: string, yes: string) => (r: Row) => r[key] ? <Badge tone="warn">{yes}</Badge> : null,
}

export { environments }
