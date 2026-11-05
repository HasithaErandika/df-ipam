import { useQuery } from '@tanstack/react-query'
import { Gauge } from 'lucide-react'
import { Badge, Button, ErrorState, Modal, Mono, SkeletonRows } from '../../components/ui'
import { get } from '../../lib/api'
import type { BlockSpace } from '../../lib/types'
import { cell, type CatalogDef, type Row } from './catalog'

const siteTypes = { site: 'Site', data_centre: 'Data centre', branch: 'Branch', cloud_region: 'Cloud region' }
const criticality = { standard: 'Standard', low: 'Low', high: 'High', critical: 'Critical' }

function FreeSpace({ row, onClose }: { row: Row; onClose: () => void }) {
  const space = useQuery({ queryKey: ['block-space', row.id], queryFn: () => get<BlockSpace>(`/network-blocks/${row.id}/free-space`) })
  return (
    <Modal open onClose={onClose} title={`Free space in ${String(row.name)}`} subtitle={<Mono>{String(row.cidr)}</Mono>} size="lg">
      {space.isPending && <SkeletonRows rows={4} />}
      {space.error && <ErrorState error={space.error} onRetry={space.refetch} />}
      {space.data && (
        <>
          <dl className="mb-4 grid grid-cols-3 gap-3">
            <div><dt className="text-[12px] text-ink-3">Used by subnets</dt><dd className="text-lg font-semibold text-ink tabular-nums">{space.data.utilisationPct}%</dd></div>
            <div><dt className="text-[12px] text-ink-3">Free addresses</dt><dd className="text-lg font-semibold text-ink tabular-nums">{BigInt(space.data.freeAddresses).toLocaleString('en-GB')}</dd></div>
            <div><dt className="text-[12px] text-ink-3">Largest free CIDR</dt><dd className="text-lg font-semibold text-ink">{space.data.largestFreeCidr ? <Mono>{space.data.largestFreeCidr}</Mono> : 'None'}</dd></div>
          </dl>
          {space.data.freeRanges.length === 0 ? <p className="text-ink-3">This block is fully used by subnets.</p> : (
            <ul className="max-h-72 overflow-y-auto">
              {space.data.freeRanges.map((r) => (
                <li key={r.first} className="flex flex-wrap justify-between gap-2 border-b border-line py-1.5 last:border-0">
                  <span className="flex flex-wrap gap-1">{r.cidrs.map((c) => <Badge key={c}><span className="font-mono">{c}</span></Badge>)}</span>
                  <span className="text-[13px] text-ink-2 tabular-nums">{BigInt(r.size).toLocaleString('en-GB')} addresses</span>
                </li>
              ))}
            </ul>
          )}
        </>
      )}
    </Modal>
  )
}

const subnetsFor = (param: string) => cell.link('Subnets', (r) => `/subnets?${param}=${r.id}`)

export const catalogs: CatalogDef[] = [
  {
    tab: 'sites', label: 'Sites', singular: 'site', path: '/sites',
    description: 'Data centres, branches and cloud regions. Zones and VLANs can belong to a site.',
    empty: 'Add the places your networks live, such as a data centre or branch.',
    fields: [
      { key: 'organisationId', label: 'Organisation', type: 'record', path: '/organisations', required: true, createOnly: true },
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'code', label: 'Code', type: 'text', required: true, hint: 'Unique within the organisation.' },
      { key: 'siteType', label: 'Type', type: 'choice', choices: siteTypes, required: true },
      { key: 'address', label: 'Address', type: 'text' },
    ],
    columns: [
      { label: 'Name', render: cell.name() }, { label: 'Code', render: cell.mono('code') },
      { label: 'Type', render: cell.choice('siteType', siteTypes) }, { label: 'Organisation', render: cell.text('organisationName') },
      { label: 'Address', render: cell.text('address') }, { label: '', render: subnetsFor('siteId') },
    ],
  },
  {
    tab: 'zones', label: 'Network zones', singular: 'network zone', path: '/network-zones',
    description: 'Security zones such as DMZ or core. A zone may belong to a site.',
    empty: 'Zones group subnets by security boundary.',
    fields: [
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'securityZone', label: 'Security zone', type: 'text' },
      { key: 'siteId', label: 'Site', type: 'record', path: '/sites' },
    ],
    columns: [{ label: 'Name', render: cell.name() }, { label: 'Security zone', render: cell.text('securityZone') }, { label: 'Site', render: cell.text('siteName') }],
  },
  {
    tab: 'vdoms', label: 'VDOMs', singular: 'VDOM', path: '/vdoms',
    description: 'Virtual domains on a firewall. A VDOM can be tied to a routing domain, and subnets mapped to it must match.',
    empty: 'A VDOM needs its firewall recorded as a device first.',
    fields: [
      { key: 'deviceId', label: 'Firewall device', type: 'record', path: '/devices', required: true },
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'routingDomainId', label: 'Routing domain', type: 'record', path: '/routing-domains' },
    ],
    columns: [
      { label: 'Name', render: cell.name() }, { label: 'Device', render: cell.text('deviceName') },
      { label: 'Routing domain', render: cell.text('routingDomainName') }, { label: '', render: subnetsFor('vdomId') },
    ],
  },
  {
    tab: 'vlans', label: 'VLANs', singular: 'VLAN', path: '/vlans', searchLabel: 'Filter by VLAN name',
    description: 'VLAN IDs 1 to 4094. The same ID can repeat at different sites or VDOMs.',
    empty: 'Add VLANs to map subnets to their layer 2 segment.',
    fields: [
      { key: 'vlanId', label: 'VLAN ID', type: 'number', required: true },
      { key: 'name', label: 'Name', type: 'text' },
      { key: 'siteId', label: 'Site', type: 'record', path: '/sites' },
      { key: 'vdomId', label: 'VDOM', type: 'record', path: '/vdoms' },
      { key: 'networkZoneId', label: 'Network zone', type: 'record', path: '/network-zones' },
    ],
    columns: [
      { label: 'VLAN', render: cell.mono('vlanId') }, { label: 'Name', render: cell.name() }, { label: 'Site', render: cell.text('siteName') },
      { label: 'VDOM', render: cell.text('vdomName') }, { label: '', render: subnetsFor('vlanId') },
    ],
  },
  {
    tab: 'blocks', label: 'Network blocks', singular: 'network block', path: '/network-blocks',
    description: 'Planning containers such as a /16 for a campus. Blocks may nest, and subnets mapped to a block must sit inside it.',
    empty: 'Blocks show how much space is left for new subnets.',
    fields: [
      { key: 'routingDomainId', label: 'Routing domain', type: 'record', path: '/routing-domains', required: true, createOnly: true },
      { key: 'cidr', label: 'CIDR', type: 'mono', required: true, createOnly: true },
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'parentId', label: 'Parent block', type: 'record', path: '/network-blocks' },
      { key: 'purpose', label: 'Purpose', type: 'text' },
    ],
    columns: [
      { label: 'CIDR', render: cell.mono('cidr') }, { label: 'Name', render: cell.name() },
      { label: 'Routing domain', render: cell.text('routingDomainName') }, { label: 'Purpose', render: cell.text('purpose') },
      { label: '', render: subnetsFor('networkBlockId') },
    ],
    actions: (row, open) => (
      <Button variant="ghost" className="h-7 px-1.5" aria-label={`Free space in ${String(row.name)}`} title="Free space"
        onClick={() => open(<FreeSpace row={row} onClose={() => open(null)} />)}><Gauge size={14} /></Button>
    ),
  },
  {
    tab: 'units', label: 'Business units', singular: 'business unit', path: '/business-units',
    description: 'Parts of the organisation that own subnets.',
    empty: 'Business units record who a subnet serves.',
    fields: [
      { key: 'organisationId', label: 'Organisation', type: 'record', path: '/organisations', required: true, createOnly: true },
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'code', label: 'Code', type: 'text', required: true },
    ],
    columns: [{ label: 'Name', render: cell.name() }, { label: 'Code', render: cell.mono('code') }, { label: 'Organisation', render: cell.text('organisationName') }],
  },
  {
    tab: 'services', label: 'Business services', singular: 'business service', path: '/business-services',
    description: 'Services such as payments or lending. Sensitive services drive stricter discovery rules later.',
    empty: 'Services link subnets and addresses to what they support.',
    fields: [
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'criticality', label: 'Criticality', type: 'choice', choices: criticality, required: true },
      { key: 'isSensitive', label: 'Sensitive service', type: 'checkbox' },
    ],
    columns: [
      { label: 'Name', render: cell.name() }, { label: 'Criticality', render: cell.choice('criticality', criticality) },
      { label: 'Sensitive', render: cell.badge('isSensitive', 'Sensitive') }, { label: '', render: subnetsFor('businessServiceId') },
    ],
  },
  {
    tab: 'applications', label: 'Applications', singular: 'application', path: '/applications',
    description: 'Applications that run on tracked addresses, optionally under a business service.',
    empty: 'Applications can be linked to IP records.',
    fields: [
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'businessServiceId', label: 'Business service', type: 'record', path: '/business-services' },
    ],
    columns: [{ label: 'Name', render: cell.name() }, { label: 'Business service', render: cell.text('businessServiceName') }],
  },
  {
    tab: 'teams', label: 'Teams', singular: 'team', path: '/teams',
    description: 'Teams that own networks, with a shared contact address.',
    empty: 'Teams make ownership clear when people move on.',
    fields: [
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'email', label: 'Email', type: 'text' },
      { key: 'description', label: 'Description', type: 'text' },
    ],
    columns: [{ label: 'Name', render: cell.name() }, { label: 'Email', render: cell.text('email') }, { label: 'Description', render: cell.text('description') }],
  },
  {
    tab: 'exceptions', label: 'Overlap exceptions', singular: 'overlap exception', path: '/overlap-exceptions',
    writePermission: 'requests:approve', searchLabel: 'Filter by reason',
    description: 'Approved reasons for a subnet to overlap another in the same routing domain. Apply one from the subnet form. Removing an exception while the overlap remains is refused.',
    empty: 'Overlaps are blocked unless an approver records an exception here.',
    fields: [
      { key: 'reason', label: 'Reason', type: 'text', required: true },
      { key: 'expiresAt', label: 'Expires', type: 'date', hint: 'Expired exceptions can no longer be applied.' },
    ],
    columns: [
      { label: 'Reason', render: cell.name('reason') }, { label: 'Approved by', render: cell.text('approvedByEmail') },
      { label: 'Approved', render: cell.date('approvedAt') }, { label: 'Expires', render: (r) => <span className="text-ink-2">{r.expiresAt ? String(r.expiresAt).slice(0, 10) : 'Never'} {r.expired ? <Badge tone="danger">Expired</Badge> : null}</span> },
      { label: 'Subnets', className: 'text-right tabular-nums', render: (r) => <span className="text-ink-2">{String(r.subnetCount ?? 0)}</span> },
    ],
  },
]
