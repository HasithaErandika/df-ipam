import { formatDateTime } from '../../lib/format'
import {  isEmail } from '../../lib/validation'
import { Badge, Mono } from '../ui'
import type { EntityDef, Row } from './types'

export const text = (key: string) => (r: Row) => r[key] || <span className="text-ink-3">—</span>

export const mono = (key: string) => (r: Row) => r[key] ? <Mono>{r[key]}</Mono> : <span className="text-ink-3">—</span>

const siteTypes: Record<string, string> = { site: 'Site or office', data_centre: 'Data centre', branch: 'Branch', cloud_region: 'Cloud region' }

const criticality: Record<string, string> = { low: 'Low', standard: 'Standard', high: 'High', critical: 'Critical' }

export const entities: EntityDef[] = [
  {
    key: 'sites', label: 'Sites', singular: 'site', path: '/sites', searchLabel: 'Search sites',
    description: 'Data centres, branches and offices.',
    fields: [
      { key: 'organisationId', label: 'Organisation', type: 'ref', ref: '/organisations', required: true, createOnly: true },
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'code', label: 'Code', type: 'text', required: true, hint: 'Short and unique, such as CMB-DC1.' },
      { key: 'siteType', label: 'Type', type: 'select', options: siteTypes },
      { key: 'address', label: 'Address', type: 'text' },
    ],
    columns: [
      { label: 'Name', render: (r) => <span className="font-medium text-ink">{r.name}</span> },
      { label: 'Code', render: (r) => <Badge>{r.code}</Badge> },
      { label: 'Type', render: (r) => siteTypes[r.siteType] ?? r.siteType ?? '—' },
      { label: 'Organisation', render: text('organisationName') },
      { label: 'Address', render: text('address') },
    ],
  },
  {
    key: 'zones', label: 'Network zones', singular: 'network zone', path: '/network-zones', searchLabel: 'Search zones',
    description: 'Security zones within a site, such as DMZ or core.',
    fields: [
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'siteId', label: 'Site', type: 'ref', ref: '/sites' },
      { key: 'securityZone', label: 'Security zone', type: 'text' },
    ],
    columns: [
      { label: 'Name', render: (r) => <span className="font-medium text-ink">{r.name}</span> },
      { label: 'Site', render: text('siteName') },
      { label: 'Security zone', render: text('securityZone') },
    ],
  },
  {
    key: 'vdoms', label: 'VDOMs', singular: 'VDOM', path: '/vdoms', searchLabel: 'Search VDOMs',
    description: 'FortiGate virtual domains, each mapped to the routing domain it routes.',
    fields: [
      { key: 'deviceId', label: 'Device', type: 'ref', ref: '/devices', required: true, createOnly: true },
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'routingDomainId', label: 'Routing domain', type: 'ref', ref: '/routing-domains', refLabel: (o) => `${o.organisationName} / ${o.name}` },
    ],
    columns: [
      { label: 'Name', render: (r) => <span className="font-medium text-ink">{r.name}</span> },
      { label: 'Device', render: text('deviceName') },
      { label: 'Routing domain', render: text('routingDomainName') },
    ],
  },
  {
    key: 'vlans', label: 'VLANs', singular: 'VLAN', path: '/vlans', searchLabel: 'Search VLANs by name',
    description: 'VLAN IDs 1 to 4094, scoped to a site or VDOM.',
    fields: [
      { key: 'vlanId', label: 'VLAN ID', type: 'number', required: true, min: 1, max: 4094 },
      { key: 'name', label: 'Name', type: 'text' },
      { key: 'siteId', label: 'Site', type: 'ref', ref: '/sites' },
      { key: 'vdomId', label: 'VDOM', type: 'ref', ref: '/vdoms', refLabel: (o) => `${o.name} (${o.deviceName ?? 'device'})` },
      { key: 'networkZoneId', label: 'Network zone', type: 'ref', ref: '/network-zones' },
    ],
    columns: [
      { label: 'VLAN', render: (r) => <Mono>{r.vlanId}</Mono> },
      { label: 'Name', render: (r) => <span className="font-medium text-ink">{r.name || '—'}</span> },
      { label: 'Site', render: text('siteName') },
      { label: 'VDOM', render: text('vdomName') },
    ],
  },
  {
    key: 'blocks', label: 'Network blocks', singular: 'network block', path: '/network-blocks', searchLabel: 'Search blocks by name',
    description: 'Planning containers. Blocks may nest; subnets are the leaf networks inside them.',
    fields: [
      { key: 'routingDomainId', label: 'Routing domain', type: 'ref', ref: '/routing-domains', required: true, createOnly: true, refLabel: (o) => `${o.organisationName} / ${o.name}` },
      { key: 'cidr', label: 'CIDR', type: 'cidr', required: true, createOnly: true },
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'parentId', label: 'Parent block', type: 'ref', ref: '/network-blocks', refLabel: (o) => `${o.cidr} ${o.name}` },
      { key: 'purpose', label: 'Purpose', type: 'text' },
    ],
    columns: [
      { label: 'CIDR', render: mono('cidr') },
      { label: 'Name', render: (r) => <span className="font-medium text-ink">{r.name}</span> },
      { label: 'Routing domain', render: text('routingDomainName') },
      { label: 'Purpose', render: text('purpose') },
    ],
  },
  {
    key: 'units', label: 'Business units', singular: 'business unit', path: '/business-units', searchLabel: 'Search business units',
    description: 'Parts of an organisation that own subnets.',
    fields: [
      { key: 'organisationId', label: 'Organisation', type: 'ref', ref: '/organisations', required: true, createOnly: true },
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'code', label: 'Code', type: 'text', required: true },
    ],
    columns: [
      { label: 'Name', render: (r) => <span className="font-medium text-ink">{r.name}</span> },
      { label: 'Code', render: (r) => <Badge>{r.code}</Badge> },
      { label: 'Organisation', render: text('organisationName') },
    ],
  },
  {
    key: 'services', label: 'Business services', singular: 'business service', path: '/business-services', searchLabel: 'Search services',
    description: 'Services the business runs. Sensitive ones default to passive discovery only.',
    fields: [
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'criticality', label: 'Criticality', type: 'select', options: criticality },
      { key: 'isSensitive', label: 'Sensitive (core banking, payments)', type: 'bool' },
    ],
    columns: [
      { label: 'Name', render: (r) => <span className="font-medium text-ink">{r.name}</span> },
      { label: 'Criticality', render: (r) => criticality[r.criticality] ?? r.criticality ?? '—' },
      { label: 'Sensitive', render: (r) => r.isSensitive ? <Badge tone="warn">Sensitive</Badge> : <span className="text-ink-3">No</span> },
    ],
  },
  {
    key: 'applications', label: 'Applications', singular: 'application', path: '/applications', searchLabel: 'Search applications',
    description: 'Applications that use addresses, grouped under business services.',
    fields: [
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'businessServiceId', label: 'Business service', type: 'ref', ref: '/business-services' },
    ],
    columns: [
      { label: 'Name', render: (r) => <span className="font-medium text-ink">{r.name}</span> },
      { label: 'Business service', render: text('businessServiceName') },
    ],
  },
  {
    key: 'teams', label: 'Teams', singular: 'team', path: '/teams', searchLabel: 'Search teams',
    description: 'Responsible teams, for ownership and notifications.',
    fields: [
      { key: 'name', label: 'Name', type: 'text', required: true },
      { key: 'email', label: 'Team email', type: 'text', check: (v) => (isEmail(v) ? undefined : 'Enter a valid email address.') },
      { key: 'description', label: 'Description', type: 'text' },
    ],
    columns: [
      { label: 'Name', render: (r) => <span className="font-medium text-ink">{r.name}</span> },
      { label: 'Email', render: text('email') },
      { label: 'Description', render: text('description') },
    ],
  },
  {
    key: 'exceptions', label: 'Overlap exceptions', singular: 'overlap exception', path: '/overlap-exceptions',
    writePermission: 'requests:approve',
    description: 'Documented reasons for a subnet to overlap another in the same routing domain. Approvers record them; the subnet then refers to one.',
    fields: [
      { key: 'reason', label: 'Reason', type: 'text', required: true },
      { key: 'expiresAt', label: 'Expires', type: 'date', hint: 'Review the overlap by this date.' },
    ],
    columns: [
      { label: 'Reason', render: (r) => <span className="text-ink">{r.reason}</span> },
      { label: 'Expires', render: (r) => r.expiresAt ? formatDateTime(r.expiresAt) : <span className="text-ink-3">No expiry</span> },
    ],
  },
]
