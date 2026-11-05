import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { MoreHorizontal, Plus, RefreshCw } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Badge, Button, Card, ConfirmDialog, EmptyState, ErrorState, Field, FormError, Input, Modal, Mono, PageHeader, Pagination, Select, SkeletonRows, Td, Th } from '../components/ui'
import { ApiError, del, get, list, post, put, queryString } from '../lib/api'
import { formatDateTime } from '../lib/format'
import { useClampPage, usePaging } from '../lib/paging'
import { useCan } from '../lib/session'
import type { Connection, ConnectorKind, SyncError, SyncJob } from '../lib/types'
import { RoutingDomainSelect } from '../components/SearchSelect'
import { useChecks, useDebounced } from '../components/workflow'
import { between, isHttpsUrl, isUUID, issues as check, required } from '../lib/validation'

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

function fieldIssues(error: unknown): Record<string, string> {
  const out: Record<string, string> = {}
  if (error instanceof ApiError) for (const d of error.details) out[d.field] = d.issue
  return out
}

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
  const needed = (options: ConnectorKind['config'], values: Record<string, string>) =>
    Object.fromEntries((options ?? []).filter((o) => o.required).map((o) => [o.key, required(values[o.key], o.label)]))
  const guid = (key: string) => (form.config[key]?.trim() && !isUUID(form.config[key]) ? 'Use the ID in its GUID form.' : undefined)
  const azure = form.kind === 'azure'
  const aws = form.kind === 'aws'
  const accessKey = form.credentials.accessKeyId?.trim() ?? ''
  const secretKey = form.credentials.secretAccessKey?.trim() ?? ''
  const checks = useChecks(check({
    name: required(form.name, 'Name'),
    baseUrl: kind?.needsUrl ? (isHttpsUrl(form.baseUrl.trim()) ? undefined : 'Use a full https URL, such as https://fw1.corp.example.') : undefined,
    syncIntervalMinutes: between(form.syncIntervalMinutes, 5, 10080, 'Sync interval'),
    ...needed(kind?.config ?? [], form.config),
    // On an edit, an empty credential keeps the stored one.
    ...(editing?.hasCredentials ? {} : needed(kind?.credentials ?? [], form.credentials)),
    ...(azure ? { tenantId: form.config.tenantId?.trim() ? guid('tenantId') : required('', 'Tenant ID'), clientId: form.config.clientId?.trim() ? guid('clientId') : required('', 'Client ID'),
      subscriptionId: form.config.subscriptionId?.trim() ? guid('subscriptionId') : required('', 'Subscription ID') } : {}),
    ...(aws ? {
      roleArn: /^arn:aws[\w-]*:iam::\d{12}:role\/[\w+=,.@/-]+$/.test(form.config.roleArn?.trim() ?? '') ? undefined : 'Use a role ARN such as arn:aws:iam::123456789012:role/df-ipam-read.',
      regions: (form.config.regions ?? '').split(',').map((r) => r.trim()).filter(Boolean).every((r) => /^[a-z]{2}(-[a-z]+)+-\d$/.test(r)) && form.config.regions?.trim() ? undefined : 'List regions such as ap-south-1, separated by commas.',
      secretAccessKey: accessKey && !secretKey ? 'Enter the secret key with its access key.' : undefined,
      accessKeyId: secretKey && !accessKey ? 'Enter the access key with its secret key.' : undefined,
    } : {}),
  }), open)
  const errors = { ...fieldIssues(save.error), ...checks.shown }
  const setMap = (group: 'config' | 'credentials', key: string, value: string) => setForm((f) => ({ ...f, [group]: { ...f[group], [key]: value } }))
  return (
    <Modal open={open} onClose={onClose} title={editing ? `Edit ${editing.name}` : 'Add a connection'} size="lg"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} disabled={!kind?.available} onClick={() => checks.submit(() => save.mutate())}>Save</Button></>}>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Connector" htmlFor="conn-kind">
          <Select id="conn-kind" value={form.kind} disabled={editing !== null} onChange={(e) => setForm({ ...blank, name: form.name, kind: e.target.value })}>
            {kinds.map((k) => <option key={k.kind} value={k.kind} disabled={!k.available}>{k.label}{k.available ? '' : ' (waiting)'}</option>)}
          </Select>
        </Field>
        <Field label="Name" htmlFor="conn-name" error={errors.name}><Input id="conn-name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></Field>
      </div>
      {kind && <p className="mt-2 text-[13px] text-ink-2">{kind.note}</p>}
      {kind?.needsUrl && (
        <div className="mt-3"><Field label="URL" htmlFor="conn-url" error={errors.baseUrl} hint="https only, on the server's approved host list."><Input id="conn-url" value={form.baseUrl} onChange={(e) => setForm({ ...form, baseUrl: e.target.value })} /></Field></div>
      )}
      <div className="mt-3 grid gap-3 sm:grid-cols-2">
        {kind?.config?.map((o) => (
          <Field key={o.key} label={o.required ? o.label : `${o.label} (optional)`} htmlFor={`conn-${o.key}`} error={errors[o.key]} hint={o.hint}>
            <Input id={`conn-${o.key}`} value={form.config[o.key] ?? ''} onChange={(e) => setMap('config', o.key, e.target.value)} />
          </Field>
        ))}
        {kind?.credentials?.map((o) => (
          <Field key={o.key} label={o.required ? o.label : `${o.label} (optional)`} htmlFor={`cred-${o.key}`} error={errors[o.key]} hint={editing?.hasCredentials ? 'Stored. Leave empty to keep it.' : undefined}>
            <Input id={`cred-${o.key}`} type="password" autoComplete="off" value={form.credentials[o.key] ?? ''} onChange={(e) => setMap('credentials', o.key, e.target.value)} />
          </Field>
        ))}
        <Field label="Sync every (minutes)" htmlFor="conn-interval" error={errors.syncIntervalMinutes}>
          <Input id="conn-interval" type="number" min={5} max={10080} value={form.syncIntervalMinutes} onChange={(e) => setForm({ ...form, syncIntervalMinutes: Number(e.target.value) })} />
        </Field>
      </div>
      <label className="mt-3 flex items-center gap-2 text-ink"><input type="checkbox" className="size-4 accent-[var(--accent)]" checked={form.isEnabled} onChange={(e) => setForm({ ...form, isEnabled: e.target.checked })} /> Sync on schedule</label>
      {Object.keys(errors).length === 0 && <div className="mt-3"><FormError error={save.error} /></div>}
      <p className="mt-3 text-[12px] text-ink-3">Credentials are encrypted and can't be read back. Use an account that can only read.</p>
    </Modal>
  )
}

function RoutingMap({ connection, onClose }: { connection: Connection | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const [map, setMap] = useState<Record<string, string>>({})
  useEffect(() => { setMap(connection?.routingDomains ?? {}) }, [connection])
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
      {contexts.length === 0 && <EmptyState title="Nothing to map yet" body="Run a sync first. The VDOMs and networks it finds appear here." />}
      <div className="space-y-2">
        {contexts.map((c) => (
          <div key={c} className="grid items-center gap-2 sm:grid-cols-2">
            <Mono>{c}</Mono>
            <RoutingDomainSelect aria-label={`Routing domain for ${c}`} value={map[c] ?? ''} onChange={(v) => setMap({ ...map, [c]: v })} emptyLabel="Not mapped" />
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
  const [search, setSearch] = useState('')
  const q = useDebounced(search.trim())
  const qs = queryString({ page: paging.page, pageSize: paging.pageSize, q })
  const { data, isPending, error, refetch } = useQuery({
    queryKey: ['connections', qs], enabled: canView, placeholderData: keepPreviousData,
    queryFn: () => list<Connection>(`/integrations/connections${qs}`),
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
