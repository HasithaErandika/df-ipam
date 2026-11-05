import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Pencil, Plug, Plus, RefreshCw } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Badge, Button, Card, ConfirmDialog, EmptyState, ErrorState, Field, FormError, Input, Modal, Mono, PageHeader, Pagination, Select, SkeletonRows, Td, Th } from '../components/ui'
import { del, get, list, listAll, post, put } from '../lib/api'
import { formatDateTime } from '../lib/format'
import { useClampPage, usePaging } from '../lib/paging'
import { useCan } from '../lib/session'
import type { Connection, ConnectorKind, RoutingDomain, SyncError, SyncJob } from '../lib/types'

const healthTone = { healthy: 'ok', degraded: 'warn', failed: 'danger', unknown: 'neutral' } as const
const healthLabel = { healthy: 'Healthy', degraded: 'Degraded', failed: 'Failed', unknown: 'Not synced yet' }
const jobTone = (s: string) => (s === 'completed' ? 'ok' as const : s === 'failed' ? 'danger' as const : 'accent' as const)

interface Form {
  kind: string
  name: string
  baseUrl: string
  config: Record<string, string>
  credentials: Record<string, string>
  syncIntervalMinutes: number
  isEnabled: boolean
}

const blank: Form = { kind: 'fortigate', name: '', baseUrl: '', config: {}, credentials: {}, syncIntervalMinutes: 60, isEnabled: true }

function ConnectionForm({ open, editing, kinds, onClose }: { open: boolean; editing: Connection | null; kinds: ConnectorKind[]; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [form, setForm] = useState<Form>(blank)
  useEffect(() => {
    if (open) setForm(editing ? { ...blank, ...editing, credentials: {} } : blank)
  }, [open, editing])
  const kind = kinds.find((k) => k.kind === form.kind)
  const save = useMutation({
    mutationFn: () => {
      // Leaving credentials empty on an edit keeps the stored ones.
      const credentials = Object.values(form.credentials).some(Boolean) ? form.credentials : undefined
      const body = { ...form, credentials, routingDomains: editing?.routingDomains ?? {} }
      return editing ? put(`/integrations/connections/${editing.id}`, body) : post('/integrations/connections', body)
    },
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['connections'] }); onClose() },
  })
  const setMap = (group: 'config' | 'credentials', key: string, value: string) => setForm((f) => ({ ...f, [group]: { ...f[group], [key]: value } }))
  return (
    <Modal open={open} onClose={onClose} title={editing ? `Edit ${editing.name}` : 'Add a connection'} size="lg"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} disabled={!kind?.available} onClick={() => save.mutate()}>Save</Button></>}>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Connector" htmlFor="conn-kind">
          <Select id="conn-kind" value={form.kind} disabled={editing !== null} onChange={(e) => setForm({ ...blank, name: form.name, kind: e.target.value })}>
            {kinds.map((k) => <option key={k.kind} value={k.kind} disabled={!k.available}>{k.label}{k.available ? '' : ' (waiting)'}</option>)}
          </Select>
        </Field>
        <Field label="Name" htmlFor="conn-name"><Input id="conn-name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></Field>
      </div>
      {kind && <p className="mt-2 text-[13px] text-ink-2">{kind.note}</p>}
      {kind?.needsUrl && (
        <div className="mt-3"><Field label="URL" htmlFor="conn-url" hint="https only, on the server's approved host list."><Input id="conn-url" value={form.baseUrl} onChange={(e) => setForm({ ...form, baseUrl: e.target.value })} /></Field></div>
      )}
      <div className="mt-3 grid gap-3 sm:grid-cols-2">
        {kind?.config?.map((o) => (
          <Field key={o.key} label={o.label} htmlFor={`conn-${o.key}`} hint={o.hint}>
            <Input id={`conn-${o.key}`} value={form.config[o.key] ?? ''} onChange={(e) => setMap('config', o.key, e.target.value)} />
          </Field>
        ))}
        {kind?.credentials?.map((o) => (
          <Field key={o.key} label={o.label} htmlFor={`cred-${o.key}`} hint={editing?.hasCredentials ? 'Stored. Leave empty to keep it.' : undefined}>
            <Input id={`cred-${o.key}`} type="password" autoComplete="off" value={form.credentials[o.key] ?? ''} onChange={(e) => setMap('credentials', o.key, e.target.value)} />
          </Field>
        ))}
        <Field label="Sync every (minutes)" htmlFor="conn-interval">
          <Input id="conn-interval" type="number" min={5} max={10080} value={form.syncIntervalMinutes} onChange={(e) => setForm({ ...form, syncIntervalMinutes: Number(e.target.value) })} />
        </Field>
      </div>
      <label className="mt-3 flex items-center gap-2 text-ink"><input type="checkbox" className="size-4 accent-[var(--accent)]" checked={form.isEnabled} onChange={(e) => setForm({ ...form, isEnabled: e.target.checked })} /> Sync on schedule</label>
      <div className="mt-3"><FormError error={save.error} /></div>
      <p className="mt-3 text-[12px] text-ink-3">Credentials are encrypted and can't be read back. Use an account that can only read.</p>
    </Modal>
  )
}

function RoutingMap({ connection, onClose }: { connection: Connection | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [map, setMap] = useState<Record<string, string>>({})
  useEffect(() => { setMap(connection?.routingDomains ?? {}) }, [connection])
  const domains = useQuery({ queryKey: ['routing-domains', 'all'], enabled: connection !== null, queryFn: () => listAll<RoutingDomain>('/routing-domains') })
  const save = useMutation({
    mutationFn: () => put(`/integrations/connections/${connection?.id}`, { ...connection, routingDomains: map }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['connections'] }); onClose() },
  })
  if (!connection) return null
  const contexts = [...new Set([...connection.contexts, ...Object.keys(connection.routingDomains)])].sort()
  return (
    <Modal open onClose={onClose} title="Routing domains" subtitle={connection.name} size="lg"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} disabled={contexts.length === 0} onClick={() => save.mutate()}>Save</Button></>}>
      <p className="mb-3 text-[13px] text-ink-2">Evidence from a place with no routing domain is kept, but isn't matched to inventory. Nothing is mapped automatically, because the same address can exist in two isolated networks.</p>
      {domains.error ? <ErrorState error={domains.error} onRetry={domains.refetch} /> : null}
      {contexts.length === 0 && <EmptyState title="Nothing to map yet" body="Run a sync first. The VDOMs and networks it finds appear here." />}
      <div className="space-y-2">
        {contexts.map((c) => (
          <div key={c} className="grid items-center gap-2 sm:grid-cols-2">
            <Mono>{c}</Mono>
            <Select aria-label={`Routing domain for ${c}`} value={map[c] ?? ''} onChange={(e) => setMap({ ...map, [c]: e.target.value })}>
              <option value="">Not mapped</option>
              {domains.data?.map((d) => <option key={d.id} value={d.id}>{d.organisationName} / {d.name}</option>)}
            </Select>
          </div>
        ))}
      </div>
      <div className="mt-3"><FormError error={save.error} /></div>
    </Modal>
  )
}

function SyncErrors({ job }: { job: SyncJob }) {
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['sync-errors', job.id], queryFn: () => list<SyncError>(`/integrations/syncs/${job.id}/errors?pageSize=50`) })
  if (isPending) return <SkeletonRows rows={2} />
  if (error) return <ErrorState error={error} onRetry={refetch} />
  return <ul className="space-y-1 p-2">{data.data.map((e, i) => <li key={i} className="text-[13px] text-ink-2"><Mono>{e.code}</Mono> {e.message}</li>)}</ul>
}

function History({ connection, onClose }: { connection: Connection | null; onClose: () => void }) {
  const paging = usePaging('hp')
  const [open, setOpen] = useState<string | null>(null)
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['syncs', connection?.id, paging.page, paging.pageSize], enabled: connection !== null, placeholderData: keepPreviousData,
    queryFn: () => list<SyncJob>(`/integrations/connections/${connection?.id}/syncs?page=${paging.page}&pageSize=${paging.pageSize}`),
    refetchInterval: (q) => (q.state.data?.data.some((j) => j.status === 'queued' || j.status === 'running') ? 5000 : false),
  })
  if (!connection) return null
  return (
    <Modal open onClose={onClose} title="Sync history" subtitle={connection.name} size="lg">
      {isPending && <SkeletonRows rows={5} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.data.length === 0 && <EmptyState title="No syncs yet" />}
      {data && data.data.length > 0 && (
        <>
          <table className="w-full border-collapse">
            <thead><tr><Th>Started</Th><Th>Status</Th><Th>Stored</Th><Th>Problems</Th></tr></thead>
            <tbody>
              {data.data.map((j) => (
                <tr key={j.id}>
                  <Td className="whitespace-nowrap text-ink-2">{formatDateTime(j.startedAt ?? j.createdAt)}</Td>
                  <Td><Badge tone={jobTone(j.status)}>{j.status}</Badge></Td>
                  <Td className="text-ink-2">{j.status === 'completed' ? `${j.counts.devices ?? 0} devices, ${j.counts.observations ?? 0} observations${j.counts.unmappedObservations ? `, ${j.counts.unmappedObservations} not mapped` : ''}` : j.error}</Td>
                  <Td>
                    {j.errorCount > 0 ? <button type="button" className="cursor-pointer text-accent hover:underline" onClick={() => setOpen(open === j.id ? null : j.id)}>{j.errorCount}</button> : '—'}
                    {open === j.id && <SyncErrors job={j} />}
                  </Td>
                </tr>
              ))}
            </tbody>
          </table>
          <Pagination meta={data.meta} onPage={paging.setPage} onPageSize={paging.setPageSize} />
        </>
      )}
    </Modal>
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
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['connections', paging.page, paging.pageSize], enabled: canView, placeholderData: keepPreviousData,
    queryFn: () => list<Connection>(`/integrations/connections?page=${paging.page}&pageSize=${paging.pageSize}`),
  })
  useClampPage(paging, data?.meta)
  const test = useMutation({
    mutationFn: (c: Connection) => post<{ ok: boolean; message: string }>(`/integrations/connections/${c.id}/test`),
    onSuccess: (r, c) => setTested({ id: c.id, ...r }),
  })
  const sync = useMutation({
    mutationFn: (c: Connection) => post(`/integrations/connections/${c.id}/sync`),
    onSuccess: (_, c) => { queryClient.invalidateQueries({ queryKey: ['syncs'] }); setHistory(c) },
  })
  const remove = useMutation({
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
        {isPending && <SkeletonRows rows={5} />}
        {error && <ErrorState error={error} onRetry={refetch} />}
        {data?.data.length === 0 && <EmptyState title="No connections yet" body="Add a FortiGate, vCenter, AWS or Azure connection." />}
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
                              <Button busy={test.isPending && test.variables?.id === c.id} onClick={() => test.mutate(c)}><Plug size={13} aria-hidden /> Test</Button>
                              <Button busy={sync.isPending && sync.variables?.id === c.id} onClick={() => sync.mutate(c)}><RefreshCw size={13} aria-hidden /> Sync</Button>
                              <Button onClick={() => setMapping(c)}>Map</Button>
                              <Button aria-label={`Edit ${c.name}`} onClick={() => setEditing(c)}><Pencil size={13} aria-hidden /></Button>
                              <Button variant="ghost" onClick={() => setRemoving(c)}>Delete</Button>
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
      {kinds.data && (
        <Card title="Available connectors" className="mt-3">
          <ul className="divide-y divide-line">
            {kinds.data.map((k) => (
              <li key={k.kind} className="flex items-start justify-between gap-3 px-4 py-2">
                <div><div className="text-[13px] font-medium text-ink">{k.label}</div><div className="text-xs text-ink-2">{k.note}</div></div>
                <Badge tone={k.available ? 'ok' : 'neutral'}>{k.available ? 'Available' : 'Waiting'}</Badge>
              </li>
            ))}
          </ul>
        </Card>
      )}
      <ConnectionForm open={adding || editing !== null} editing={editing} kinds={kinds.data ?? []} onClose={() => { setAdding(false); setEditing(null) }} />
      <RoutingMap connection={mapping} onClose={() => setMapping(null)} />
      <History connection={history} onClose={() => setHistory(null)} />
      <ConfirmDialog open={removing !== null} title="Delete this connection?" body="Only a connection with no sync history can be deleted. Disable it otherwise, because its history is evidence."
        confirmLabel="Delete" busy={remove.isPending} error={remove.error} onConfirm={() => remove.mutate()} onCancel={() => setRemoving(null)} />
    </>
  )
}
