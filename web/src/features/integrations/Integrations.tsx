import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { MoreHorizontal, Plus, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { Badge, Button, Card, ConfirmDialog, EmptyState, ErrorState, FormError, Input, PageHeader, Pagination, SkeletonRows, Td, Th } from '../../components/ui'
import { useDebounced } from '../../components/workflow'
import { ConnectionForm, RoutingMap } from './ConnectionForm'
import { History } from './History'
import { healthLabel, healthTone } from './shared'
import { del, get, list, post, queryString } from '../../lib/api'
import { formatDateTime } from '../../lib/format'
import { useClampPage, usePaging } from '../../lib/paging'
import { useCan } from '../../lib/session'
import type { Connection, ConnectorKind } from '../../lib/types'

function RowMenu({ label, actions }: { label: string; actions: { label: string; run: () => void; danger?: boolean }[] }) {
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild><Button aria-label={`More actions for ${label}`}><MoreHorizontal size={14} aria-hidden /></Button></DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content align="end" sideOffset={4} className="z-40 min-w-44 rounded-[5px] border border-line-strong bg-surface py-1 shadow-[0_8px_24px_-8px_rgba(11,18,32,.25)]">
          {actions.map((a) => (
            <DropdownMenu.Item key={a.label} onSelect={a.run} className={`cursor-pointer px-3 py-1.5 text-[13px] outline-none data-[highlighted]:bg-surface-2 ${a.danger ? 'text-danger' : 'text-ink'}`}>{a.label}</DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

export function Integrations() {
  const queryClient = useQueryClient()
  const canManage = useCan('integrations:manage')
  const canAudit = useCan('audit:view')
  const canView = canManage || canAudit
  const paging = usePaging()
  const [editing, setEditing] = useState<Connection | null>(null)
  const [adding, setAdding] = useState(false)
  const [mapping, setMapping] = useState<Connection | null>(null)
  const [history, setHistory] = useState<Connection | null>(null)
  const [removing, setRemoving] = useState<Connection | null>(null)
  const [tested, setTested] = useState<{ id: string; ok: boolean; message: string } | null>(null)
  const kinds = useQuery({ queryKey: ['connector-kinds'], enabled: canView, queryFn: () => get<ConnectorKind[]>('/integrations/kinds') })
  const [search, setSearch] = useState('')
  const q = useDebounced(search.trim())
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, q })
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['connections', qs], enabled: canView, placeholderData: keepPreviousData,
    queryFn: () => list<Connection>(`/integrations/connections${qs}`),
  })
  useClampPage(paging, data?.meta)
  const test = useMutation({ meta: { inlineError: true },
    mutationFn: (c: Connection) => post<{ ok: boolean; message: string }>(`/integrations/connections/${c.id}/test`),
    onSuccess: (r, c) => setTested({ id: c.id, ...r }),
  })
  const sync = useMutation({ meta: { success: 'Sync queued.', inlineError: true },
    mutationFn: (c: Connection) => post(`/integrations/connections/${c.id}/sync`),
    onSuccess: (_, c) => { queryClient.invalidateQueries({ queryKey: ['syncs'] }); setHistory(c) },
  })
  const remove = useMutation({ meta: { success: 'Connection deleted.', inlineError: true },
    mutationFn: () => del(`/integrations/connections/${removing?.id}`),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['connections'] }); setRemoving(null) },
  })
  const label = (kind: string) => kinds.data?.find((k) => k.kind === kind)?.label ?? kind
  if (!canView) return <><PageHeader title="Integrations" /><Card><EmptyState title="You don't have access to integrations" /></Card></>
  return (
    <>
      <PageHeader title="Integrations" description="Read-only connections to infrastructure platforms. What they report is evidence, and never changes ownership or allocation."
        actions={canManage && <Button variant="primary" onClick={() => setAdding(true)}><Plus size={14} aria-hidden /> Add connection</Button>} />
      {test.error ? <div className="mb-3"><FormError error={test.error} /></div> : null}
      {sync.error ? <div className="mb-3"><FormError error={sync.error} /></div> : null}
      <Card>
        <div className="border-b border-line p-2.5"><Input aria-label="Search connections" className="max-w-xs" placeholder="Search by name or URL" value={search} onChange={(e) => { setSearch(e.target.value); paging.setPage(1) }} /></div>
        {isPending && <SkeletonRows rows={5} />}
        {error && <ErrorState error={error} onRetry={refetch} />}
        {data?.data.length === 0 && (q ? <EmptyState title="No connections match" /> : <EmptyState title="No connections yet" body="Add a FortiGate, vCenter, AWS or Azure connection." />)}
        {data && data.data.length > 0 && (
          <>
            <div className="overflow-x-auto">
              <table className="w-full border-collapse">
                <thead><tr><Th>Connection</Th><Th>Platform</Th><Th>Health</Th><Th>Last success</Th><Th>Mapped</Th><Th className="text-right">Actions</Th></tr></thead>
                <tbody>
                  {data.data.map((c) => {
                    const mapped = c.contexts.filter((x) => c.routingDomains[x]).length
                    return (
                      <tr key={c.id} className="align-top hover:bg-surface-2">
                        <Td>
                          <button type="button" className="cursor-pointer font-medium text-ink hover:text-accent" onClick={() => setHistory(c)}>{c.name}</button>
                          {!c.isEnabled && <span className="ml-2"><Badge>Disabled</Badge></span>}
                          {c.lastError && <div className="mt-0.5 max-w-md text-xs text-danger">{c.lastError}</div>}
                          {tested?.id === c.id && <div className={`mt-0.5 max-w-md text-xs ${tested.ok ? 'text-ok' : 'text-danger'}`}>{tested.message}</div>}
                        </Td>
                        <Td className="text-ink-2">{label(c.kind)}</Td>
                        <Td><Badge tone={healthTone[c.health]}>{healthLabel[c.health]}</Badge></Td>
                        <Td className="whitespace-nowrap text-ink-2">{c.lastSuccessAt ? formatDateTime(c.lastSuccessAt) : 'Never'}</Td>
                        <Td className="whitespace-nowrap text-ink-2">{c.contexts.length === 0 ? '—' : `${mapped} of ${c.contexts.length}`}</Td>
                        <Td className="text-right whitespace-nowrap">
                          {canManage && (
                            <span className="inline-flex gap-1">
                              <Button busy={sync.isPending && sync.variables?.id === c.id} onClick={() => sync.mutate(c)}><RefreshCw size={13} aria-hidden /> Sync</Button>
                              <RowMenu label={c.name} actions={[
                                { label: 'Test connection', run: () => test.mutate(c) },
                                { label: 'Map routing domains', run: () => setMapping(c) },
                                { label: 'Sync history', run: () => setHistory(c) },
                                { label: 'Edit', run: () => setEditing(c) },
                                { label: 'Delete', run: () => setRemoving(c), danger: true },
                              ]} />
                            </span>
                          )}
                        </Td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
            <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
          </>
        )}
      </Card>
      <ConnectionForm open={adding || editing !== null} editing={editing} kinds={kinds.data ?? []} onClose={() => { setAdding(false); setEditing(null) }} />
      <RoutingMap connection={mapping} onClose={() => setMapping(null)} />
      <History connection={history} onClose={() => setHistory(null)} />
      <ConfirmDialog open={removing !== null} title="Delete this connection?" body="Only a connection with no sync history can be deleted. Disable it otherwise, because its history is evidence."
        confirmLabel="Delete" busy={remove.isPending} error={remove.error} onConfirm={() => remove.mutate()} onCancel={() => setRemoving(null)} />
    </>
  )
}
