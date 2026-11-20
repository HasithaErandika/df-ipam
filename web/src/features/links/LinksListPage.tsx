import { Archive, ArchiveRestore, Eye, Pencil, Plus, Upload } from 'lucide-react'
import { Link, useMatch, useNavigate } from 'react-router'
import { Button, Card, DataTable, FilterBar, FilterSelect, none, PageHeader, RowMenu, SearchInput, Stack, StateBadge, type Column } from '../../components/ui'
import { useFilters, usePagedList } from '../../lib/listing'
import { useCan } from '../../lib/session'
import type { NetworkLink } from '../../lib/types'
import { ExportMenu } from '../shared/FileTransfer'
import { ColumnChooser, useColumnChoice } from './ColumnChooser'
import { LinkFormModal } from './LinkFormModal'
import { bandwidthLine, criticalities, formatDay, formatMoney, linkRoles, linkStatuses, linkStatusTone, typeName } from './labels'
import { useArchive } from './useArchive'
import { useLinkSettings, useProviders } from './queries'

const filterKeys = ['q', 'linkType', 'providerId', 'site', 'status', 'criticality', 'archived', 'sort'] as const

const optional = [
  { key: 'destination', label: 'Destination' }, { key: 'bandwidth', label: 'Bandwidth' }, { key: 'role', label: 'Link role' },
  { key: 'rental', label: 'Monthly rental' }, { key: 'criticality', label: 'Criticality' }, { key: 'circuit', label: 'Circuit ID' },
  { key: 'publicIps', label: 'Public IPs' }, { key: 'contractEnd', label: 'Contract expiry' }, { key: 'officer', label: 'Responsible officer' },
]

const sorts = [
  { value: 'name', text: 'Name' }, { value: 'type', text: 'Type' }, { value: 'provider', text: 'Provider' }, { value: 'site', text: 'Location' },
  { value: '-bandwidth', text: 'Fastest first' }, { value: 'status', text: 'Status' }, { value: 'criticality', text: 'Most critical first' },
  { value: '-updated', text: 'Recently updated' },
]

const moneySorts = [{ value: '-rental', text: 'Highest rental first' }, { value: 'contractEnd', text: 'Contract ending soonest' }]

/** Every network link, with search, filters, a column chooser and export. */
export function LinksListPage() {
  const navigate = useNavigate()
  const adding = Boolean(useMatch('/network-links/new'))
  const settings = useLinkSettings()
  const providers = useProviders()
  const f = useFilters(filterKeys)
  const { query, rows, meta, paging } = usePagedList<NetworkLink>('/network-links', f.values)
  const can = { create: useCan('links:create'), edit: useCan('links:edit'), archive: useCan('links:archive'), money: useCan('links:commercial_view'), io: useCan('links:import_export') }
  const cols = useColumnChoice('links.columns', ['destination', 'bandwidth', 'role', 'rental'])
  const archive = useArchive()
  const show = (k: string) => cols.shown.includes(k) && (can.money || !['rental', 'contractEnd'].includes(k))

  const columns: Column<NetworkLink>[] = [
    { key: 'code', header: 'Link ID', cell: (l) => <Link to={`/network-links/${l.id}`} className="font-mono text-[12.5px] text-accent hover:underline">{l.code}</Link> },
    { key: 'name', header: 'Link name', cell: (l) => <Stack main={<Link to={`/network-links/${l.id}`} className="font-medium text-ink hover:text-accent hover:underline">{l.name}</Link>} sub={l.archivedAt ? 'Archived' : undefined} /> },
    { key: 'type', header: 'Link type', hideBelow: 'md', cell: (l) => typeName(settings.data, l.linkType) },
    { key: 'provider', header: 'Service provider', hideBelow: 'lg', cell: (l) => l.providerName ?? none },
    { key: 'site', header: 'Source location', hideBelow: 'lg', cell: (l) => l.sourceSite },
    ...(show('destination') ? [{ key: 'destination', header: 'Destination', hideBelow: 'xl' as const, cell: (l: NetworkLink) => l.destination || none }] : []),
    ...(show('bandwidth') ? [{ key: 'bandwidth', header: 'Bandwidth', hideBelow: 'xl' as const, cell: (l: NetworkLink) => bandwidthLine(l.downMbps, l.upMbps) || none }] : []),
    ...(show('role') ? [{ key: 'role', header: 'Link role', hideBelow: 'xl' as const, cell: (l: NetworkLink) => linkRoles[l.linkRole] ?? l.linkRole }] : []),
    ...(show('criticality') ? [{ key: 'criticality', header: 'Criticality', cell: (l: NetworkLink) => criticalities[l.criticality] }] : []),
    ...(show('circuit') ? [{ key: 'circuit', header: 'Circuit ID', cell: (l: NetworkLink) => (l.circuitId ? <span className="font-mono text-[12.5px]">{l.circuitId}</span> : none) }] : []),
    ...(show('publicIps') ? [{ key: 'publicIps', header: 'Public IPs', cell: (l: NetworkLink) => (l.publicIps ? <span className="font-mono text-[12.5px]">{l.publicIps.replace(/\/32$/, '')}</span> : none) }] : []),
    ...(show('officer') ? [{ key: 'officer', header: 'Responsible officer', cell: (l: NetworkLink) => l.officerName ?? l.officerEmail ?? none }] : []),
    ...(show('rental') ? [{ key: 'rental', header: 'Monthly rental', className: 'text-right tabular-nums', cell: (l: NetworkLink) => (l.noSeparateCost ? <span className="text-ink-3">No separate cost</span> : l.monthlyRental == null ? none : formatMoney(l.monthlyRental, l.currency)) }] : []),
    ...(show('contractEnd') ? [{ key: 'contractEnd', header: 'Contract expiry', cell: (l: NetworkLink) => (l.contractEnd ? formatDay(l.contractEnd) : none) }] : []),
    { key: 'status', header: 'Status', cell: (l) => <StateBadge tone={linkStatusTone[l.status]}>{linkStatuses[l.status]}</StateBadge> },
    { key: 'actions', header: '', className: 'text-right', cell: (l) => (
      <RowMenu actions={[
        { label: 'View', icon: <Eye size={14} aria-hidden />, onSelect: () => navigate(`/network-links/${l.id}`) },
        { label: 'Edit', icon: <Pencil size={14} aria-hidden />, onSelect: () => navigate(`/network-links/${l.id}/edit`), hidden: !can.edit || Boolean(l.archivedAt) },
        { label: l.archivedAt ? 'Restore' : 'Archive', icon: l.archivedAt ? <ArchiveRestore size={14} aria-hidden /> : <Archive size={14} aria-hidden />, onSelect: () => void archive(l), hidden: !can.archive, danger: !l.archivedAt },
      ]} />
    ) },
  ]

  const types = (settings.data?.types ?? []).map((t) => ({ value: t.key, text: t.label }))
  const exportFilters = Object.fromEntries(Object.entries(f.values).filter(([k, v]) => v && k !== 'sort'))
  return (
    <>
      <PageHeader title="All network links" description="Internet lines, VPN tunnels, leased lines and third-party connections. Status is administrative: it says whether a link is in service, not whether it is up right now."
        actions={<>
          {can.io && <Button onClick={() => navigate('/network-links/import-export')}><Upload size={14} aria-hidden />Import</Button>}
          {can.io && <ExportMenu path="/network-links/export" params={exportFilters} name="network-links" />}
          {can.create && <Button variant="primary" onClick={() => navigate('/network-links/new')}><Plus size={14} aria-hidden />Add network link</Button>}
        </>} />
      <Card>
        <FilterBar active={f.active} onClear={f.clear} end={<ColumnChooser columns={optional.filter((c) => can.money || !['rental', 'contractEnd'].includes(c.key))} shown={cols.shown} onToggle={cols.toggle} />}>
          <SearchInput value={f.values.q} onChange={(v) => f.set('q', v)} placeholder="Search name, ID, circuit, provider, IP" />
          <FilterSelect label="Type" value={f.values.linkType} onChange={(v) => f.set('linkType', v)} options={types} />
          <FilterSelect label="Provider" value={f.values.providerId} onChange={(v) => f.set('providerId', v)} options={(providers.data ?? []).map((p) => ({ value: p.id, text: p.name }))} />
          <FilterSelect label="Location" value={f.values.site} onChange={(v) => f.set('site', v)} options={(settings.data?.sites ?? []).map((s) => ({ value: s, text: s }))} />
          <FilterSelect label="Status" value={f.values.status} onChange={(v) => f.set('status', v)} options={Object.entries(linkStatuses).map(([value, text]) => ({ value, text }))} />
          <FilterSelect label="Criticality" value={f.values.criticality} onChange={(v) => f.set('criticality', v)} options={Object.entries(criticalities).map(([value, text]) => ({ value, text }))} />
          <FilterSelect label="Archived" value={f.values.archived} onChange={(v) => f.set('archived', v)} anyLabel="Not archived" options={[{ value: 'only', text: 'Archived only' }, { value: 'all', text: 'Both' }]} />
          <FilterSelect label="Sort" value={f.values.sort} onChange={(v) => f.set('sort', v)} anyLabel="Link ID" options={[...sorts, ...(can.money ? moneySorts : [])]} />
        </FilterBar>
        <DataTable columns={columns} rows={rows} query={query} rowKey={(l) => l.id} meta={meta} paging={paging} filtered={f} pageSizes={[10, 25, 50, 100]}
          empty={{ title: 'No network links yet', body: 'Add your internet lines, VPN tunnels and leased lines one at a time, or import them from Excel.',
            action: can.create ? <Button variant="primary" onClick={() => navigate('/network-links/new')}><Plus size={14} aria-hidden />Add network link</Button> : undefined }} />
      </Card>
      <LinkFormModal open={adding} onClose={() => navigate('/network-links/all')} onSaved={(id) => navigate(`/network-links/${id}`)} />
    </>
  )
}
