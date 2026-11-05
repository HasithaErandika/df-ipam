import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { Badge, Button, Card, ConfirmDialog, EmptyState, ErrorState, Mono, Pagination, SkeletonRows, Td, Th } from '../../components/ui'
import { del } from '../../lib/api'
import { domainKinds, formatCount } from '../../lib/format'
import { useClampPage, usePaging } from '../../lib/paging'
import { useCan } from '../../lib/session'
import type { Organisation, RoutingDomain } from '../../lib/types'
import { useDomains, useOrganisations } from './hooks'
import { EditDomain, EditOrganisation, NewDomain, NewOrganisation } from './modals'

export function Hierarchy() {
  const queryClient = useQueryClient()
  const canCreate = useCan('inventory:create')
  const canEdit = useCan('inventory:edit')
  const canDelete = useCan('inventory:delete')
  const orgPaging = usePaging('org')
  const domainPaging = usePaging('domain')
  const organisations = useOrganisations(orgPaging)
  const domains = useDomains(domainPaging)
  useClampPage(orgPaging, organisations.data?.meta)
  useClampPage(domainPaging, domains.data?.meta)
  const [adding, setAdding] = useState<'org' | 'domain' | null>(null)
  const [editOrg, setEditOrg] = useState<Organisation | null>(null)
  const [editDomain, setEditDomain] = useState<RoutingDomain | null>(null)
  const [removing, setRemoving] = useState<{ path: string; label: string } | null>(null)
  const remove = useMutation({ meta: { success: 'Deleted.', inlineError: true },
    mutationFn: (path: string) => del(path),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['organisations'] }); queryClient.invalidateQueries({ queryKey: ['routing-domains'] }); setRemoving(null) },
  })
  const hasOrgs = (organisations.data?.data.length ?? 0) > 0
  const icon = 'h-7 px-1.5'
  return (
    <>
      <div className="grid gap-4 xl:grid-cols-3">
        <Card title="Organisations" action={canCreate && <Button variant="ghost" className="h-7 px-2" onClick={() => setAdding('org')}><Plus size={15} aria-hidden /> Add</Button>}>
          {organisations.isPending && <SkeletonRows rows={3} />}
          {organisations.error && <ErrorState error={organisations.error} onRetry={organisations.refetch} />}
          {organisations.data?.data.length === 0 && <EmptyState title="No organisations yet" body="Start here: subnets belong to an organisation's routing domain." />}
          {hasOrgs && (
            <ul>
              {organisations.data?.data.map((o) => (
                <li key={o.id} className="flex items-center justify-between gap-2 border-b border-line px-4 py-2 last:border-0">
                  <span className="font-medium text-ink">{o.name}</span>
                  <span className="flex items-center gap-1">
                    <Badge>{o.code}</Badge>
                    {canEdit && <Button variant="ghost" className={icon} aria-label={`Edit ${o.name}`} onClick={() => setEditOrg(o)}><Pencil size={14} /></Button>}
                    {canDelete && <Button variant="ghost" className={icon} aria-label={`Delete ${o.name}`} onClick={() => { remove.reset(); setRemoving({ path: `/organisations/${o.id}`, label: o.name }) }}><Trash2 size={14} /></Button>}
                  </span>
                </li>
              ))}
            </ul>
          )}
          {organisations.data && hasOrgs && <Pagination meta={organisations.data.meta} onPage={orgPaging.setPage} />}
        </Card>
        <Card title="Routing domains" className="xl:col-span-2" action={canCreate && hasOrgs && <Button variant="ghost" className="h-7 px-2" onClick={() => setAdding('domain')}><Plus size={15} aria-hidden /> Add</Button>}>
          {domains.isPending && <SkeletonRows rows={3} />}
          {domains.error && <ErrorState error={domains.error} onRetry={domains.refetch} />}
          {domains.data?.data.length === 0 && <EmptyState title="No routing domains yet" body="Each organisation gets a Default routing domain." />}
          {domains.data && domains.data.data.length > 0 && (
            <div className="overflow-x-auto">
              <table className="w-full border-collapse">
                <thead><tr><Th>Name</Th><Th>Organisation</Th><Th>Type</Th><Th>RD</Th><Th className="text-right">Subnets</Th>{(canEdit || canDelete) && <Th />}</tr></thead>
                <tbody>
                  {domains.data.data.map((d) => (
                    <tr key={d.id}>
                      <Td className="font-medium text-ink">{d.name} {d.isIsolated && <Badge tone="warn">Isolated</Badge>}</Td>
                      <Td className="text-ink-2">{d.organisationName}</Td>
                      <Td className="text-ink-2">{domainKinds[d.kind]}</Td>
                      <Td className="text-ink-2">{d.routeDistinguisher ? <Mono>{d.routeDistinguisher}</Mono> : '—'}</Td>
                      <Td className="text-right text-ink-2 tabular-nums">{formatCount(d.subnetCount)}</Td>
                      {(canEdit || canDelete) && (
                        <Td className="text-right whitespace-nowrap">
                          {canEdit && <Button variant="ghost" className={icon} aria-label={`Edit ${d.name}`} onClick={() => setEditDomain(d)}><Pencil size={14} /></Button>}
                          {canDelete && <Button variant="ghost" className={icon} aria-label={`Delete ${d.name}`} onClick={() => { remove.reset(); setRemoving({ path: `/routing-domains/${d.id}`, label: d.name }) }}><Trash2 size={14} /></Button>}
                        </Td>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {domains.data && domains.data.data.length > 0 && <Pagination meta={domains.data.meta} onPage={domainPaging.setPage} onPageSize={domainPaging.setPageSize} />}
        </Card>
      </div>
      <NewOrganisation open={adding === 'org'} onClose={() => setAdding(null)} />
      <NewDomain open={adding === 'domain'} onClose={() => setAdding(null)} />
      {editOrg && <EditOrganisation org={editOrg} onClose={() => setEditOrg(null)} />}
      {editDomain && <EditDomain domain={editDomain} onClose={() => setEditDomain(null)} />}
      <ConfirmDialog open={removing !== null} title={`Delete ${removing?.label}?`}
        body="Only records nothing else uses can be deleted. Archive subnets or remove related records first."
        confirmLabel="Delete" busy={remove.isPending} error={remove.error} onConfirm={() => removing && remove.mutate(removing.path)} onCancel={() => setRemoving(null)} />
    </>
  )
}
