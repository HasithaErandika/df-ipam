import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Search, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { del, list, queryString } from '../../lib/api'
import { useClampPage, usePaging } from '../../lib/paging'
import { useCan } from '../../lib/session'
import { Button, Card, ConfirmDialog, EmptyState, ErrorState, Input, Pagination, SkeletonRows, Td, Th } from '../ui'
import { useDebounced } from '../workflow'
import { EditModal } from './EditModal'
import type { EntityDef, Row } from './types'

/** Lists, creates, edits and deletes one kind of reference record. */
export function CatalogManager({ def }: { def: EntityDef }) {
  const queryClient = useQueryClient()
  const canCreate = useCan(def.writePermission ?? 'inventory:create')
  const canEdit = useCan(def.writePermission ?? 'inventory:edit')
  const canDelete = useCan(def.writePermission ?? 'inventory:delete')
  const writable = !def.readOnly
  const paging = usePaging(def.key)
  const [search, setSearch] = useState('')
  const q = useDebounced(search.trim())
  const [editing, setEditing] = useState<Row | 'new' | null>(null)
  const [removing, setRemoving] = useState<Row | null>(null)
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, q })
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['catalog', def.key, qs], placeholderData: keepPreviousData, queryFn: () => list<Row>(`${def.path}${qs}`),
  })
  useClampPage(paging, data?.meta)
  const remove = useMutation({ meta: { success: `${def.singular} deleted.`, inlineError: true },
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
