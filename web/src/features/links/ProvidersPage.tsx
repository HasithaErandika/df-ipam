import { Pencil, Plus } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router'
import { Button, Card, DataTable, Details, Modal, none, PageHeader, RowMenu, Stack, StateBadge, type Column } from '../../components/ui'
import { useDialogParam } from '../../lib/dialogParam'
import { useCan } from '../../lib/session'
import type { LinkProvider } from '../../lib/types'
import { ProviderForm } from './ProviderForm'
import { useProviders } from './queries'

/** The companies that provide links, with their contacts. */
export function ProvidersPage() {
  const q = useProviders()
  const canManage = useCan('links:providers')
  const [editing, setEditing] = useState<{ provider?: LinkProvider } | null>(null)
  const [openId, setOpen] = useDialogParam('provider')
  const rows = q.data ?? []
  const open = rows.find((p) => p.id === openId) ?? null

  const columns: Column<LinkProvider>[] = [
    { key: 'name', header: 'Provider', cell: (p) => <button type="button" className="cursor-pointer font-medium text-ink hover:text-accent hover:underline" onClick={() => setOpen(p.id)}>{p.name}</button> },
    { key: 'manager', header: 'Account manager', hideBelow: 'md', cell: (p) => <Stack main={p.accountManager || none} sub={[p.contactNumber, p.email].filter(Boolean).join(' · ') || undefined} /> },
    { key: 'support', header: 'Support', hideBelow: 'lg', cell: (p) => <Stack main={p.supportHotline || none} sub={p.supportEmail || undefined} /> },
    { key: 'links', header: 'Links', className: 'tabular-nums', cell: (p) => (p.links ? <Link className="text-accent hover:underline" to={`/network-links/all?providerId=${p.id}`}>{p.links}</Link> : '0') },
    { key: 'status', header: 'Status', cell: (p) => <StateBadge tone={p.active ? 'ok' : 'neutral'}>{p.active ? 'Active' : 'Inactive'}</StateBadge> },
    { key: 'actions', header: '', className: 'text-right', cell: (p) => (canManage ? <RowMenu actions={[{ label: 'Edit', icon: <Pencil size={14} aria-hidden />, onSelect: () => setEditing({ provider: p }) }]} /> : null) },
  ]
  return (
    <>
      <PageHeader title="Service providers" description="Contacts here are used for every link from the provider unless a link has its own. Providers are switched off, never deleted, so links keep them."
        actions={canManage && <Button variant="primary" onClick={() => setEditing({})}><Plus size={14} aria-hidden />Add provider</Button>} />
      <Card>
        <DataTable columns={columns} rows={rows} query={q} rowKey={(p) => p.id}
          empty={{ title: 'No providers yet', body: 'Add the companies that provide your internet lines, leased lines and other links.' }} />
      </Card>
      <ProviderForm open={editing !== null} provider={editing?.provider} onClose={() => setEditing(null)} />
      <Modal open={open !== null} onClose={() => setOpen(null)} title={open?.name ?? ''} subtitle={open?.active ? 'Active' : 'Inactive'}
        footer={<>{canManage && open && <Button onClick={() => { setOpen(null); setEditing({ provider: open }) }}><Pencil size={14} aria-hidden />Edit</Button>}<Button variant="ghost" onClick={() => setOpen(null)}>Close</Button></>}>
        {open && (
          <div className="space-y-3">
            <Details items={[
              ['Account manager', open.accountManager || null], ['Contact number', open.contactNumber || null], ['Email', open.email || null],
              ['Support hotline', open.supportHotline || null], ['Support email', open.supportEmail || null],
              ['Website', open.website ? <a className="text-accent hover:underline" href={open.website} target="_blank" rel="noreferrer noopener">{open.website}</a> : null],
            ]} />
            {open.remarks && <p className="text-[13px] whitespace-pre-wrap text-ink-2">{open.remarks}</p>}
            <Link className="text-[13px] text-accent hover:underline" to={`/network-links/all?providerId=${open.id}`}>See its {open.links} link{open.links === 1 ? '' : 's'}</Link>
          </div>
        )}
      </Modal>
    </>
  )
}
