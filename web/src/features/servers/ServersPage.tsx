import { useMutation } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useState } from 'react'
import { Button, Card, DataTable, FilterBar, FilterSelect, PageHeader, PathTabs, SearchInput, Segmented, useConfirm } from '../../components/ui'
import { del } from '../../lib/api'
import { ipTypes, options, registerStatuses } from '../../lib/labels'
import { useFilters, usePagedList } from '../../lib/listing'
import { inventoryKeys, useEnvironments, useInvalidate, useSegments } from '../../lib/queries'
import { useCan } from '../../lib/session'
import { useRouteTab } from '../../lib/tabs'
import type { RegisterEntry } from '../../lib/types'
import { ExportMenu, ImportButton } from '../shared/FileTransfer'
import { RegisterIPModal } from '../shared/RegisterIPModal'
import { registerColumns, serverColumns } from './registerColumns'

const keys = ['segmentId', 'environmentId', 'status', 'health', 'ipType', 'q', 'sort'] as const
const importColumns = ['ip', 'status', 'ipType', 'environment', 'assignedName', 'platform', 'services', 'expectedMac', 'owner', 'department', 'monitor', 'critical', 'checkPort', 'notes']

function useRelease() {
  const confirm = useConfirm()
  const refresh = useInvalidate(...inventoryKeys)
  const release = useMutation({
    mutationFn: (r: RegisterEntry) => del(`/ip-addresses/${r.id}`),
    meta: { success: (r: RegisterEntry) => `${r.ip} released.` },
    onSuccess: () => void refresh(),
  })
  return async (r: RegisterEntry) => {
    if (await confirm({ title: `Release ${r.ip}?`, body: `The register entry${r.assignedName ? ` for “${r.assignedName}”` : ''} is removed and the address becomes unassigned. The audit log keeps what it held. Nothing on the network changes.`, confirmLabel: 'Release address' })) {
      release.mutate(r)
    }
  }
}

export function ServersPage() {
  const tab = useRouteTab(['servers', 'register'])
  const servers = tab === 'servers'
  const canEdit = useCan('inventory:edit')
  const segments = useSegments()
  const environments = useEnvironments()
  const f = useFilters(keys)
  const params = { ...f.values, view: servers ? 'servers' : undefined }
  const { query, rows, meta, paging } = usePagedList<RegisterEntry>('/ip-addresses', params, { refetchInterval: servers ? 60_000 : undefined })
  const [editing, setEditing] = useState<RegisterEntry | null>(null)
  const [adding, setAdding] = useState(false)
  const release = useRelease()
  const actions = { canEdit, onEdit: setEditing, onRelease: (r: RegisterEntry) => void release(r) }
  const envOptions = [{ value: '', text: 'All' }, ...(environments.data ?? []).map((e) => ({ value: e.id, text: e.name }))]

  return (
    <>
      <PageHeader title="Servers" description="The register is the official record. Monitored entries are checked by ping, or a TCP port when ping is blocked."
        actions={<>
          <ExportMenu path="/ip-addresses/export" params={Object.fromEntries(Object.entries(params).filter(([, v]) => v)) as Record<string, string>} name={servers ? 'servers' : 'ip-register'} />
          {canEdit && (servers
            ? <ImportButton path="/ip-addresses/import?as=server" title="Import servers" columns={importColumns} required="ip" refreshKeys={inventoryKeys}
                notes={<>A blank <code className="font-mono">ipType</code> means Server. A blank <code className="font-mono">monitor</code> or <code className="font-mono">critical</code> takes the environment's default; <code className="font-mono">environment</code> is a name from Settings, or the segment's default. Services are codes separated by spaces, such as <code className="font-mono">nginx docker</code>. An export from this page imports back as it is.</>} />
            : <ImportButton path="/ip-addresses/import" title="Import IP register" columns={importColumns} required="ip" refreshKeys={inventoryKeys} />)}
          {canEdit && <Button variant="primary" onClick={() => setAdding(true)}><Plus size={14} aria-hidden />{servers ? 'Add server' : 'Add address'}</Button>}
        </>} />
      <PathTabs base="/servers" tabs={[{ key: 'servers', label: 'Servers' }, { key: 'register', label: 'IP register' }]} />
      <Card>
        {servers && environments.data && environments.data.length > 0 && (
          <div className="border-b border-line px-3 py-2.5"><Segmented label="Environment" value={f.values.environmentId} onChange={(v) => f.set('environmentId', v)} options={envOptions} /></div>
        )}
        <FilterBar active={f.active} onClear={f.clear}>
          <SearchInput value={f.values.q} onChange={(v) => f.set('q', v)} placeholder="Search IP, name, owner, MAC" />
          {servers
            ? <FilterSelect label="Status" value={f.values.health} onChange={(v) => f.set('health', v)} anyLabel="Any status" options={[{ value: 'up', text: 'Up' }, { value: 'down', text: 'Down' }, { value: 'unknown', text: 'Unknown' }]} />
            : <>
                <FilterSelect label="Segment" value={f.values.segmentId} onChange={(v) => f.set('segmentId', v)} options={(segments.data ?? []).map((s) => ({ value: s.id, text: s.name }))} />
                <FilterSelect label="Register status" value={f.values.status} onChange={(v) => f.set('status', v)} anyLabel="Any status" options={options(registerStatuses)} />
                <FilterSelect label="Type" value={f.values.ipType} onChange={(v) => f.set('ipType', v)} options={options(ipTypes)} />
                <FilterSelect label="Environment" value={f.values.environmentId} onChange={(v) => f.set('environmentId', v)} options={envOptions.slice(1)} />
              </>}
          <FilterSelect label="Sort" value={f.values.sort} onChange={(v) => f.set('sort', v)} anyLabel="By IP" options={[{ value: 'name', text: 'By name' }, { value: 'health', text: 'By status' }, { value: 'segment', text: 'By segment' }, { value: '-updated', text: 'Recently changed' }]} />
        </FilterBar>
        <DataTable columns={servers ? serverColumns(actions) : registerColumns(actions)} rows={rows} query={query} rowKey={(r) => r.id} meta={meta} paging={paging} filtered={f}
          rowClassName={(r) => (r.monitor && r.health === 'down' ? 'bg-danger-soft/40' : undefined)}
          empty={servers
            ? { title: 'No servers yet', body: 'Add a server with a suggested Available candidate IP, or register entries in a server or cloud segment.' }
            : { title: 'The register is empty', body: 'Add addresses by hand, import a CSV or Excel file, or register what Discovery finds.' }} />
      </Card>
      <RegisterIPModal open={adding} serverOnly={servers} prefill={servers ? { ipType: 'server' } : undefined} onClose={() => setAdding(false)} />
      <RegisterIPModal open={editing !== null} entry={editing ?? undefined} onClose={() => setEditing(null)} />
    </>
  )
}
