import { type EntityDef } from '../../components/catalog/types'
import { Badge, Mono } from '../../components/ui'
import { formatDateTime } from '../../lib/format'
import { isCron } from '../../lib/validation'

const routingDomain = { key: 'routingDomainId', label: 'Routing domain', type: 'ref' as const, ref: '/routing-domains', refLabel: (o: Record<string, string>) => `${o.organisationName} / ${o.name}` }

export const exclusions: EntityDef = {
  key: 'exclusions', label: 'Exclusions', singular: 'exclusion', path: '/discovery/exclusions', writePermission: 'discovery:manage',
  description: 'Ranges discovery never touches, whatever a policy allows. Exclusions always win, even on jobs already running.',
  fields: [{ key: 'target', label: 'Network', type: 'cidr', required: true }, { ...routingDomain, hint: 'Leave empty to exclude it in every routing domain.' },
    { key: 'reason', label: 'Reason', type: 'text', required: true }, { key: 'expiresAt', label: 'Expires', type: 'date' }],
  columns: [{ label: 'Network', render: (r) => <Mono>{r.target}</Mono> }, { label: 'Routing domain', render: (r) => r.routingDomainName ?? 'All' },
    { label: 'Reason', render: (r) => r.reason }, { label: 'Expires', render: (r) => (r.expiresAt ? formatDateTime(r.expiresAt) : 'Never') }],
}

export const blackouts: EntityDef = {
  key: 'blackouts', label: 'Blackouts', singular: 'blackout', path: '/discovery/blackouts', writePermission: 'discovery:manage', searchLabel: 'Search blackouts',
  description: 'Maintenance periods when discovery leaves the network alone, or everything if no network is given. Targets of running jobs are skipped too.',
  fields: [{ key: 'name', label: 'Name', type: 'text', required: true }, { key: 'target', label: 'Network', type: 'cidr' },
    { key: 'startsAt', label: 'Starts', type: 'datetime', required: true, hint: 'In your own time zone.' },
    { key: 'endsAt', label: 'Ends', type: 'datetime', required: true }, { key: 'reason', label: 'Reason', type: 'text' }],
  validate: (f) => ({ endsAt: f.startsAt && f.endsAt && String(f.endsAt) <= String(f.startsAt) ? 'The end must be after the start.' : undefined }),
  columns: [{ label: 'Name', render: (r) => r.name }, { label: 'Network', render: (r) => (r.target ? <Mono>{r.target}</Mono> : 'Everything') },
    { label: 'From', render: (r) => formatDateTime(r.startsAt) }, { label: 'To', render: (r) => formatDateTime(r.endsAt) }],
}

export const schedules: EntityDef = {
  key: 'schedules', label: 'Schedules', singular: 'schedule', path: '/discovery/schedules', writePermission: 'discovery:manage',
  description: 'Recurring discovery of one subnet under an approved policy. The worker starts a job when the schedule matches, unless the last one is still running or the emergency stop is on.',
  fields: [{ key: 'policyId', label: 'Policy', type: 'ref', ref: '/discovery/policies', required: true },
    { key: 'subnetId', label: 'Subnet', type: 'ref', ref: '/subnets', required: true, refLabel: (o) => `${o.calculated?.cidr ?? ''} ${o.name}` },
    { key: 'cron', label: 'Schedule (cron)', type: 'text', required: true, hint: 'minute hour day month weekday, such as 30 23 * * 1-5',
      check: (v) => (isCron(v) ? undefined : 'Use five fields: minute hour day month weekday.') },
    { key: 'timezone', label: 'Timezone', type: 'timezone', hint: 'Defaults to Asia/Colombo.' },
    { key: 'agentId', label: 'Agent', type: 'ref', ref: '/agents' }, { key: 'priority', label: 'Priority (1 high to 9 low)', type: 'number', min: 1, max: 9 },
    { key: 'isEnabled', label: 'Enabled', type: 'bool' }],
  columns: [{ label: 'Policy', render: (r) => r.policyName }, { label: 'Subnet', render: (r) => r.subnetName }, { label: 'Cron', render: (r) => <Mono>{r.cron}</Mono> },
    { label: 'Last run', render: (r) => (r.lastRunAt ? formatDateTime(r.lastRunAt) : 'Never') }, { label: 'Enabled', render: (r) => (r.isEnabled ? <Badge tone="ok">On</Badge> : <Badge>Off</Badge>) }],
}
