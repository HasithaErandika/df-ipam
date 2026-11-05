import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Search, Trash2 } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { ApiError, del, list, listAll, patchPlain, post, queryString } from '../lib/api'
import { formatDateTime } from '../lib/format'
import { useClampPage, usePaging } from '../lib/paging'
import { useCan } from '../lib/session'
import { Badge, Button, Card, ConfirmDialog, EmptyState, ErrorState, Field, FormError, Input, Modal, Mono, Pagination, Select, SkeletonRows, Td, Th } from './ui'
import { useDebounced } from './workflow'

/* eslint-disable @typescript-eslint/no-explicit-any */
export type Row = Record<string, any> & { id: string }

export interface FieldDef {
  key: string
  label: string
  type: 'text' | 'number' | 'bool' | 'cidr' | 'ip' | 'ref' | 'select' | 'date'
  required?: boolean
  /** For ref fields: the list endpoint, and how to name an option. */
  ref?: string
  refLabel?: (o: Row) => string
  options?: Record<string, string>
  hint?: string
  /** Fixed once created, because other records depend on it. */
  createOnly?: boolean
}

export interface ColumnDef { label: string; render: (r: Row) => ReactNode; className?: string }

export interface EntityDef {
  key: string
  label: string
  singular: string
  path: string
  description: string
  fields: FieldDef[]
  columns: ColumnDef[]
  /** Permission to write, when it isn't the inventory permissions. */
  writePermission?: string
  readOnly?: boolean
  searchLabel?: string
  rowAction?: (r: Row) => ReactNode
}

function RefSelect({ field, id, value, onChange, invalid }: { field: FieldDef; id: string; value: string; onChange: (v: string) => void; invalid?: boolean }) {
  const options = useQuery({ queryKey: ['options', field.ref], queryFn: () => listAll<Row>(field.ref!) })
  return (
    <Select id={id} aria-invalid={invalid} value={value} onChange={(e) => onChange(e.target.value)}>
      <option value="">{field.required ? 'Select' : 'None'}</option>
      {options.data?.map((o) => <option key={o.id} value={o.id}>{field.refLabel ? field.refLabel(o) : o.name ?? o.email}</option>)}
    </Select>
  )
}

function toForm(def: EntityDef, row: Row | null): Record<string, string | boolean> {
  const out: Record<string, string | boolean> = {}
  for (const f of def.fields) {
    const v = row?.[f.key]
    if (f.type === 'bool') out[f.key] = Boolean(v)
    else if (f.type === 'date') out[f.key] = typeof v === 'string' ? v.slice(0, 10) : ''
    else out[f.key] = v === null || v === undefined ? '' : String(v)
  }
  return out
}

function toBody(def: EntityDef, form: Record<string, string | boolean>, editing: boolean) {
  const body: Record<string, unknown> = {}
  for (const f of def.fields) {
    if (editing && f.createOnly) continue
    const v = form[f.key]
    if (f.type === 'bool') body[f.key] = Boolean(v)
    else if (f.type === 'number') body[f.key] = v === '' ? null : Number(v)
    else if (v === '' && f.type !== 'text') body[f.key] = editing ? '' : undefined
    else body[f.key] = v
  }
  return body
}

function EditModal({ def, row, onClose }: { def: EntityDef; row: Row | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const editing = row !== null
  const [form, setForm] = useState(() => toForm(def, row))
  const save = useMutation({
    mutationFn: () => {
      const body = toBody(def, form, editing)
      return editing ? patchPlain(`${def.path}/${row.id}`, body) : post(def.path, body)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['catalog', def.key] })
      queryClient.invalidateQueries({ queryKey: ['options', def.path] })
      onClose()
    },
  })
  const errors: Record<string, string> = {}
  if (save.error instanceof ApiError) for (const d of save.error.details) errors[d.field] = d.issue
  const set = (key: string, v: string | boolean) => setForm((f) => ({ ...f, [key]: v }))
  return (
    <Modal open onClose={onClose} title={`${editing ? 'Edit' : 'New'} ${def.singular}`}
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" type="submit" form={`form-${def.key}`} busy={save.isPending}>{editing ? 'Save changes' : `Create ${def.singular}`}</Button></>}>
      <form id={`form-${def.key}`} noValidate onSubmit={(e) => { e.preventDefault(); save.mutate() }} className="space-y-3.5">
        {def.fields.map((f) => {
          const id = `${def.key}-${f.key}`
          const label = `${f.label}${f.required ? '' : ' (optional)'}`
          const disabled = editing && f.createOnly
          if (f.type === 'bool') {
            return (
              <label key={f.key} className="flex items-center gap-2 text-[13px] text-ink">
                <input type="checkbox" checked={Boolean(form[f.key])} onChange={(e) => set(f.key, e.target.checked)} /> {f.label}
              </label>
            )
          }
          return (
            <Field key={f.key} label={label} htmlFor={id} error={errors[f.key]} hint={disabled ? 'Fixed once created.' : f.hint}>
              {f.type === 'ref' ? (
                disabled ? <Input id={id} disabled value={row?.[`${f.key.replace(/Id$/, '')}Name`] ?? String(form[f.key])} />
                  : <RefSelect field={f} id={id} value={String(form[f.key])} onChange={(v) => set(f.key, v)} invalid={Boolean(errors[f.key])} />
              ) : f.type === 'select' ? (
                <Select id={id} value={String(form[f.key])} disabled={disabled} onChange={(e) => set(f.key, e.target.value)}>
                  <option value="">{f.required ? 'Select' : 'None'}</option>
                  {Object.entries(f.options ?? {}).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
                </Select>
              ) : (
                <Input id={id} disabled={disabled} aria-invalid={Boolean(errors[f.key])}
                  type={f.type === 'number' ? 'number' : f.type === 'date' ? 'date' : 'text'}
                  className={f.type === 'cidr' || f.type === 'ip' ? 'font-mono' : undefined} spellCheck={f.type === 'text'}
                  value={String(form[f.key])} onChange={(e) => set(f.key, e.target.value)} />
              )}
            </Field>
          )
        })}
        {Object.keys(errors).length === 0 && <FormError error={save.error} />}
      </form>
    </Modal>
  )
}

/** Lists, creates, edits and deletes one kind of reference record. */
export function CatalogManager({ def }: { def: EntityDef }) {
  const queryClient = useQueryClient()
  const canCreate = useCan(def.writePermission ?? 'inventory:create')
  const canEdit = useCan(def.writePermission ?? 'inventory:edit')
  const canDelete = useCan(def.writePermission ?? 'inventory:delete')
  const writable = !def.readOnly
  const paging = usePaging(def.key, 25)
  const [search, setSearch] = useState('')
  const q = useDebounced(search.trim())
  const [editing, setEditing] = useState<Row | 'new' | null>(null)
  const [removing, setRemoving] = useState<Row | null>(null)
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, q })
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['catalog', def.key, qs], placeholderData: keepPreviousData, queryFn: () => list<Row>(`${def.path}${qs}`),
  })
  useClampPage(paging, data?.meta)
  const remove = useMutation({
    mutationFn: (id: string) => del(`${def.path}/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['catalog', def.key] })
      queryClient.invalidateQueries({ queryKey: ['options', def.path] })
      setRemoving(null)
    },
  })
  const showActions = writable && (canEdit || canDelete)
  return (
    <Card title={def.label} action={writable && canCreate && <Button variant="primary" className="h-7" onClick={() => setEditing('new')}><Plus size={14} aria-hidden /> New {def.singular}</Button>}>
      <p className="border-b border-line px-4 py-2 text-[12.5px] text-ink-2">{def.description}</p>
      {def.searchLabel && (
        <div className="border-b border-line p-2.5">
          <div className="relative max-w-sm">
            <Search size={14} className="absolute top-1/2 left-2.5 -translate-y-1/2 text-ink-3" aria-hidden />
            <Input aria-label={def.searchLabel} placeholder={def.searchLabel} className="pl-8" value={search} onChange={(e) => { setSearch(e.target.value); paging.setPage(1) }} />
          </div>
        </div>
      )}
      {isPending && <SkeletonRows rows={4} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && (q ? <EmptyState title={`No ${def.label.toLowerCase()} match`} />
        : <EmptyState title={`No ${def.label.toLowerCase()} yet`} body={def.readOnly ? 'These arrive from read-only sources.' : undefined} />)}
      {data && data.data.length > 0 && (
        <>
          <div className="overflow-x-auto">
            <table className="w-full border-collapse">
              <thead><tr>{def.columns.map((c) => <Th key={c.label} className={c.className}>{c.label}</Th>)}{(showActions || def.rowAction) && <Th />}</tr></thead>
              <tbody>
                {data.data.map((r) => (
                  <tr key={r.id} className="hover:bg-surface-2">
                    {def.columns.map((c) => <Td key={c.label} className={c.className}>{c.render(r)}</Td>)}
                    {(showActions || def.rowAction) && (
                      <Td className="text-right whitespace-nowrap">
                        {def.rowAction?.(r)}
                        {writable && canEdit && <Button variant="ghost" className="h-7 px-1.5" aria-label={`Edit ${def.singular}`} onClick={() => setEditing(r)}><Pencil size={14} /></Button>}
                        {writable && canDelete && <Button variant="ghost" className="h-7 px-1.5" aria-label={`Delete ${def.singular}`} onClick={() => { remove.reset(); setRemoving(r) }}><Trash2 size={14} /></Button>}
                      </Td>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
        </>
      )}
      {editing && <EditModal def={def} row={editing === 'new' ? null : editing} onClose={() => setEditing(null)} />}
      <ConfirmDialog open={removing !== null} title={`Delete this ${def.singular}?`}
        body="Records that still use it block the delete. The change is recorded in the audit log."
        confirmLabel="Delete" busy={remove.isPending} error={remove.error} onConfirm={() => removing && remove.mutate(removing.id)} onCancel={() => setRemoving(null)} />
    </Card>
  )
}

const text = (key: string) => (r: Row) => r[key] || <span className="text-ink-3">—</span>
const mono = (key: string) => (r: Row) => r[key] ? <Mono>{r[key]}</Mono> : <span className="text-ink-3">—</span>

export const siteTypes: Record<string, string> = { data_centre: 'Data centre', branch: 'Branch', office: 'Office', cloud: 'Cloud', other: 'Other' }
export const criticality: Record<string, string> = { low: 'Low', medium: 'Medium', high: 'High', critical: 'Critical' }

export const entities: EntityDef[] = [
  {
    key: 'sites', label: 'Sites', singular: 'site', path: '/sites', searchLabel: 'Search sites',
    description: 'Data centres, branches and offices.',
    fields: [
      { key: 'organisationId', label: 'Organisation', type: 'ref', ref: '/organisations', required: true, createOnly: true },
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'code', label: 'Code', type: 'text', required: true, hint: 'Short and unique, such as CMB-DC1.' },
      { key: 'siteType', label: 'Type', type: 'select', options: siteTypes },
      { key: 'address', label: 'Address', type: 'text' },
    ],
    columns: [
      { label: 'Name', render: (r) => <span className="font-medium text-ink">{r.name}</span> },
      { label: 'Code', render: (r) => <Badge>{r.code}</Badge> },
      { label: 'Type', render: (r) => siteTypes[r.siteType] ?? r.siteType ?? '—' },
      { label: 'Organisation', render: text('organisationName') },
      { label: 'Address', render: text('address') },
    ],
  },
  {
    key: 'zones', label: 'Network zones', singular: 'network zone', path: '/network-zones', searchLabel: 'Search zones',
    description: 'Security zones within a site, such as DMZ or core.',
    fields: [
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'siteId', label: 'Site', type: 'ref', ref: '/sites' },
      { key: 'securityZone', label: 'Security zone', type: 'text' },
    ],
    columns: [
      { label: 'Name', render: (r) => <span className="font-medium text-ink">{r.name}</span> },
      { label: 'Site', render: text('siteName') },
      { label: 'Security zone', render: text('securityZone') },
    ],
  },
  {
    key: 'vdoms', label: 'VDOMs', singular: 'VDOM', path: '/vdoms', searchLabel: 'Search VDOMs',
    description: 'FortiGate virtual domains, each mapped to the routing domain it routes.',
    fields: [
      { key: 'deviceId', label: 'Device', type: 'ref', ref: '/devices', required: true, createOnly: true },
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'routingDomainId', label: 'Routing domain', type: 'ref', ref: '/routing-domains', refLabel: (o) => `${o.organisationName} / ${o.name}` },
    ],
    columns: [
      { label: 'Name', render: (r) => <span className="font-medium text-ink">{r.name}</span> },
      { label: 'Device', render: text('deviceName') },
      { label: 'Routing domain', render: text('routingDomainName') },
    ],
  },
  {
    key: 'vlans', label: 'VLANs', singular: 'VLAN', path: '/vlans', searchLabel: 'Search VLANs by name',
    description: 'VLAN IDs 1 to 4094, scoped to a site or VDOM.',
    fields: [
      { key: 'vlanId', label: 'VLAN ID', type: 'number', required: true },
      { key: 'name', label: 'Name', type: 'text' },
      { key: 'siteId', label: 'Site', type: 'ref', ref: '/sites' },
      { key: 'vdomId', label: 'VDOM', type: 'ref', ref: '/vdoms', refLabel: (o) => `${o.name} (${o.deviceName ?? 'device'})` },
      { key: 'networkZoneId', label: 'Network zone', type: 'ref', ref: '/network-zones' },
    ],
    columns: [
      { label: 'VLAN', render: (r) => <Mono>{r.vlanId}</Mono> },
      { label: 'Name', render: (r) => <span className="font-medium text-ink">{r.name || '—'}</span> },
      { label: 'Site', render: text('siteName') },
      { label: 'VDOM', render: text('vdomName') },
    ],
  },
  {
    key: 'blocks', label: 'Network blocks', singular: 'network block', path: '/network-blocks', searchLabel: 'Search blocks by name',
    description: 'Planning containers. Blocks may nest; subnets are the leaf networks inside them.',
    fields: [
      { key: 'routingDomainId', label: 'Routing domain', type: 'ref', ref: '/routing-domains', required: true, createOnly: true, refLabel: (o) => `${o.organisationName} / ${o.name}` },
      { key: 'cidr', label: 'CIDR', type: 'cidr', required: true, createOnly: true },
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'parentId', label: 'Parent block', type: 'ref', ref: '/network-blocks', refLabel: (o) => `${o.cidr} ${o.name}` },
      { key: 'purpose', label: 'Purpose', type: 'text' },
    ],
    columns: [
      { label: 'CIDR', render: mono('cidr') },
      { label: 'Name', render: (r) => <span className="font-medium text-ink">{r.name}</span> },
      { label: 'Routing domain', render: text('routingDomainName') },
      { label: 'Purpose', render: text('purpose') },
    ],
  },
  {
    key: 'units', label: 'Business units', singular: 'business unit', path: '/business-units', searchLabel: 'Search business units',
    description: 'Parts of an organisation that own subnets.',
    fields: [
      { key: 'organisationId', label: 'Organisation', type: 'ref', ref: '/organisations', required: true, createOnly: true },
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'code', label: 'Code', type: 'text', required: true },
    ],
    columns: [
      { label: 'Name', render: (r) => <span className="font-medium text-ink">{r.name}</span> },
      { label: 'Code', render: (r) => <Badge>{r.code}</Badge> },
      { label: 'Organisation', render: text('organisationName') },
    ],
  },
  {
    key: 'services', label: 'Business services', singular: 'business service', path: '/business-services', searchLabel: 'Search services',
    description: 'Services the business runs. Sensitive ones default to passive discovery only.',
    fields: [
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'criticality', label: 'Criticality', type: 'select', options: criticality },
      { key: 'isSensitive', label: 'Sensitive (core banking, payments)', type: 'bool' },
    ],
    columns: [
      { label: 'Name', render: (r) => <span className="font-medium text-ink">{r.name}</span> },
      { label: 'Criticality', render: (r) => criticality[r.criticality] ?? r.criticality ?? '—' },
      { label: 'Sensitive', render: (r) => r.isSensitive ? <Badge tone="warn">Sensitive</Badge> : <span className="text-ink-3">No</span> },
    ],
  },
  {
    key: 'applications', label: 'Applications', singular: 'application', path: '/applications', searchLabel: 'Search applications',
    description: 'Applications that use addresses, grouped under business services.',
    fields: [
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'businessServiceId', label: 'Business service', type: 'ref', ref: '/business-services' },
    ],
    columns: [
      { label: 'Name', render: (r) => <span className="font-medium text-ink">{r.name}</span> },
      { label: 'Business service', render: text('businessServiceName') },
    ],
  },
  {
    key: 'teams', label: 'Teams', singular: 'team', path: '/teams', searchLabel: 'Search teams',
    description: 'Responsible teams, for ownership and notifications.',
    fields: [
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'email', label: 'Team email', type: 'text' },
      { key: 'description', label: 'Description', type: 'text' },
    ],
    columns: [
      { label: 'Name', render: (r) => <span className="font-medium text-ink">{r.name}</span> },
      { label: 'Email', render: text('email') },
      { label: 'Description', render: text('description') },
    ],
  },
  {
    key: 'exceptions', label: 'Overlap exceptions', singular: 'overlap exception', path: '/overlap-exceptions',
    writePermission: 'requests:approve',
    description: 'Documented reasons for a subnet to overlap another in the same routing domain. Approvers record them; the subnet then refers to one.',
    fields: [
      { key: 'reason', label: 'Reason', type: 'text', required: true },
      { key: 'expiresAt', label: 'Expires', type: 'date', hint: 'Review the overlap by this date.' },
    ],
    columns: [
      { label: 'Reason', render: (r) => <span className="text-ink">{r.reason}</span> },
      { label: 'Expires', render: (r) => r.expiresAt ? formatDateTime(r.expiresAt) : <span className="text-ink-3">No expiry</span> },
    ],
  },
]
