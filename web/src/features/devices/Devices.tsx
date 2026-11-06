import { useRouteTab } from '../../lib/tabs'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { GitMerge } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router'
import { CatalogManager } from '../../components/catalog/CatalogManager'
import { type EntityDef } from '../../components/catalog/types'
import { DeviceIcon, deviceTypes, OSBadge, osFamilies } from '../../components/devices'
import { Badge, Button, Card, ConfirmDialog, EmptyState, ErrorState, Mono, PageHeader, SkeletonRows, Td, Th } from '../../components/ui'
import { RouteTabs } from '../../components/workflow'
import { get, post } from '../../lib/api'
import { environments } from '../../lib/format'
import { useCan } from '../../lib/session'

const options = (m: Record<string, { label: string }>) => Object.fromEntries(Object.entries(m).map(([k, v]) => [k, v.label]))

const deviceDef: EntityDef = {
  key: 'devices', label: 'Devices', singular: 'device', path: '/devices', searchLabel: 'Search devices by name',
  ownerKey: 'ownerId', ownerLocked: ['ownerId', 'businessServiceId', 'sensitivity', 'status'],
  description: 'One record per machine, however many sources report it. Device type and OS come from people, imports, integrations and discovery evidence; nothing is guessed.',
  fields: [
    { key: 'name', label: 'Name', type: 'text', required: true },
    { key: 'deviceType', label: 'Type', type: 'select', options: options(deviceTypes) },
    { key: 'hostname', label: 'Hostname', type: 'text' },
    { key: 'managementIp', label: 'Management IP', type: 'ip' },
    { key: 'vendor', label: 'Vendor', type: 'text' },
    { key: 'model', label: 'Model', type: 'text' },
    { key: 'serialNumber', label: 'Serial number', type: 'text' },
    { key: 'osFamily', label: 'OS family', type: 'select', options: { unknown: 'Unknown', ...options(osFamilies) } },
    { key: 'osVersion', label: 'OS version', type: 'text' },
    { key: 'siteId', label: 'Site', type: 'ref', ref: '/sites' },
    { key: 'environment', label: 'Environment', type: 'select', options: environments },
    { key: 'ownerId', label: 'Owner', type: 'ref', ref: '/people', refLabel: (o) => o.email },
    { key: 'businessServiceId', label: 'Business service', type: 'ref', ref: '/business-services' },
    { key: 'status', label: 'Status', type: 'select', options: { active: 'Active', inactive: 'Inactive', retired: 'Retired', unknown: 'Unknown' } },
    { key: 'sensitivity', label: 'Sensitivity', type: 'select', options: { standard: 'Standard', sensitive: 'Sensitive', restricted: 'Restricted' } },
    { key: 'notes', label: 'Notes', type: 'text' },
  ],
  columns: [
    { label: 'Device', render: (r) => (
      <span className="flex items-center gap-2"><DeviceIcon type={r.deviceType} />
        <span><span className="block font-medium text-ink">{r.name}</span><span className="block text-[11.5px] text-ink-3">{deviceTypes[r.deviceType]?.label ?? r.deviceType}{r.vendor && ` · ${r.vendor}`}{r.model && ` ${r.model}`}</span></span>
      </span>) },
    { label: 'OS', render: (r) => <OSBadge family={r.osFamily === 'unknown' ? null : r.osFamily} version={r.osVersion || null} /> },
    { label: 'Management IP', render: (r) => (r.managementIp ? <Mono>{r.managementIp}</Mono> : '—') },
    { label: 'Site', render: (r) => r.siteName ?? '—' },
    { label: 'Status', render: (r) => <Badge tone={r.status === 'active' ? 'ok' : 'neutral'}>{r.status}</Badge> },
  ],
  rowAction: (r) => <Link to={`/search?q=${encodeURIComponent(r.name)}`} className="mr-1 text-[12.5px] text-accent hover:underline">Addresses</Link>,
}

const interfaceDef: EntityDef = {
  key: 'interfaces', label: 'Interfaces', singular: 'interface', path: '/interfaces', searchLabel: 'Search interfaces by name',
  description: 'Interfaces across all devices. IP records link to them, so an address shows the port and VLAN it lives on.',
  fields: [
    { key: 'deviceId', label: 'Device', type: 'ref', ref: '/devices', required: true, createOnly: true },
    { key: 'name', label: 'Name', type: 'text', required: true },
    { key: 'mac', label: 'MAC', type: 'text' },
    { key: 'interfaceType', label: 'Type', type: 'text', hint: 'Such as ethernet, vlan, aggregate, tunnel.' },
    { key: 'adminStatus', label: 'Admin status', type: 'select', options: { up: 'Up', down: 'Down', unknown: 'Unknown' } },
    { key: 'operStatus', label: 'Operational status', type: 'select', options: { up: 'Up', down: 'Down', unknown: 'Unknown' } },
    { key: 'vlanId', label: 'VLAN', type: 'ref', ref: '/vlans', refLabel: (o) => `${o.vlanId} ${o.name}` },
    { key: 'vdomId', label: 'VDOM', type: 'ref', ref: '/vdoms' },
    { key: 'routingDomainId', label: 'Routing domain', type: 'ref', ref: '/routing-domains', refLabel: (o) => `${o.organisationName} / ${o.name}` },
    { key: 'description', label: 'Description', type: 'text' },
  ],
  columns: [
    { label: 'Interface', render: (r) => <span className="font-medium text-ink">{r.name}</span> },
    { label: 'Device', render: (r) => r.deviceName ?? '—' },
    { label: 'MAC', render: (r) => (r.mac ? <Mono>{r.mac}</Mono> : '—') },
    { label: 'Status', render: (r) => <span className="text-ink-2">{r.adminStatus} / {r.operStatus}</span> },
    { label: 'Description', render: (r) => r.description || '—' },
  ],
}

interface Dup { reason: string; value: string; devices: { id: string; name: string; deviceType: string; vendor: string; sourceSystem: string; managementIp: string | null; interfaces: number; addresses: number }[] }
const reasons: Record<string, string> = { serial_number: 'Same serial number', interface_mac: 'Same interface MAC', management_ip: 'Same management IP', hostname: 'Same hostname' }

function Duplicates() {
  const queryClient = useQueryClient()
  const canEdit = useCan('inventory:edit')
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['catalog', 'devices', 'duplicates'], queryFn: () => get<Dup[]>('/devices/duplicates') })
  const [merging, setMerging] = useState<{ from: Dup['devices'][number]; into: Dup['devices'][number] } | null>(null)
  const merge = useMutation({ meta: { success: 'Devices merged.', inlineError: true },
    mutationFn: () => post(`/devices/${merging?.from.id}/merge`, { intoId: merging?.into.id }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['catalog'] }); setMerging(null) },
  })
  return (
    <Card title="Possible duplicates">
      <p className="border-b border-line px-4 py-2 text-[12.5px] text-ink-2">Devices that look like one machine reported by several sources. Merging keeps one record and moves interfaces and address links to it.</p>
      {isPending && <SkeletonRows rows={4} />}
      {error && <ErrorState error={error} onRetry={refetch} />}
      {data?.length === 0 && <EmptyState title="No duplicates found" />}
      {data?.map((g) => (
        <div key={g.reason + g.value} className="border-b border-line px-4 py-3 last:border-0">
          <div className="mb-1.5 text-[13px]"><Badge tone="warn">{reasons[g.reason] ?? g.reason}</Badge> <Mono>{g.value}</Mono></div>
          <table className="w-full border-collapse">
            <thead><tr><Th>Device</Th><Th>Source</Th><Th>Management IP</Th><Th className="text-right">Interfaces</Th><Th className="text-right">Addresses</Th>{canEdit && <Th />}</tr></thead>
            <tbody>{g.devices.map((d, i) => (
              <tr key={d.id}>
                <Td><span className="flex items-center gap-2"><DeviceIcon type={d.deviceType} />{d.name}</span></Td>
                <Td className="text-ink-2">{d.sourceSystem}</Td>
                <Td>{d.managementIp ? <Mono>{d.managementIp}</Mono> : '—'}</Td>
                <Td className="text-right tabular-nums">{d.interfaces}</Td>
                <Td className="text-right tabular-nums">{d.addresses}</Td>
                {canEdit && <Td className="text-right">{g.devices.length > 1 &&
                  <Button variant="ghost" className="h-7" onClick={() => { merge.reset(); setMerging({ from: d, into: g.devices[i === 0 ? 1 : 0] }) }}><GitMerge size={14} aria-hidden /> Merge into {g.devices[i === 0 ? 1 : 0].name}</Button>}</Td>}
              </tr>
            ))}</tbody>
          </table>
        </div>
      ))}
      <ConfirmDialog open={merging !== null} title={`Merge ${merging?.from.name} into ${merging?.into.name}?`}
        body={`${merging?.from.name} is removed. Its interfaces and address links move to ${merging?.into.name}, and empty fields there are filled from it. The audit log keeps the removed record.`}
        confirmLabel="Merge" busy={merge.isPending} error={merge.error} onConfirm={() => merge.mutate()} onCancel={() => setMerging(null)} />
    </Card>
  )
}

export function Devices() {
  const tab = useRouteTab(['devices', 'duplicates'])
  return (
    <>
      <PageHeader title="Device Inventory" description="Firewalls, routers, switches, access points, servers, VMs, laptops, PCs, phones and more, each with its interfaces and addresses." />
      <RouteTabs tabs={[{ key: 'devices', label: 'Devices' }, { key: 'duplicates', label: 'Possible duplicates' }]} active={tab} base="/devices" />
      {tab === 'devices' ? <CatalogManager def={deviceDef} /> : <Duplicates />}
    </>
  )
}

export function Interfaces() {
  return (
    <>
      <PageHeader title="Interface Inventory" description="Search interfaces across all devices, with their status, VLAN, VDOM and routing context." />
      <CatalogManager def={interfaceDef} />
    </>
  )
}
