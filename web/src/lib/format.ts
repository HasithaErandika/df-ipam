export const environments: Record<string, string> = {
  production: 'Production',
  disaster_recovery: 'Disaster Recovery',
  uat: 'UAT',
  development: 'Development',
  testing: 'Testing',
}

export const domainKinds: Record<string, string> = {
  default: 'Default', vrf: 'VRF', vpc: 'AWS VPC', vnet: 'Azure VNet', isolated: 'Isolated',
}

/** Address counts can exceed safe integers, so they stay strings. */
export function formatCapacity(count: string, family: 4 | 6, prefixLength: number): string {
  if (family === 6 && prefixLength < 96) return `2^${128 - prefixLength}`
  return BigInt(count).toLocaleString('en-GB')
}

export function formatCount(n: number): string {
  return n.toLocaleString('en-GB')
}

export function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString('en-GB', { dateStyle: 'medium', timeStyle: 'short' })
}

export type Severity = 'normal' | 'warning' | 'critical'

export function severity(pct: number, warn: number, critical: number): Severity {
  if (pct >= critical) return 'critical'
  return pct >= warn ? 'warning' : 'normal'
}

export const classifications: Record<string, string> = {
  new_device: 'New device',
  unknown_ip: 'Unknown IP',
  ip_conflict_suspected: 'Conflict suspected',
  duplicate_mac: 'Duplicate MAC',
  dns_mismatch: 'DNS mismatch',
  allocated_not_observed: 'Allocated, not observed',
  observed_not_allocated: 'Observed, not allocated',
  subnet_overlap: 'Subnet overlap',
  ownership_missing: 'Ownership missing',
  infrastructure_mismatch: 'Infrastructure mismatch',
  stale_evidence: 'Stale evidence',
}

/** Page numbers to show, with gaps marked by null: 1 … 4 5 6 … 20. */
export function pageWindow(page: number, totalPages: number): (number | null)[] {
  const wanted = new Set([1, totalPages, page - 1, page, page + 1].filter((p) => p >= 1 && p <= totalPages))
  const sorted = [...wanted].sort((a, b) => a - b)
  const out: (number | null)[] = []
  sorted.forEach((p, i) => {
    if (i > 0) {
      const gap = p - sorted[i - 1]
      if (gap === 2) out.push(p - 1)
      else if (gap > 2) out.push(null)
    }
    out.push(p)
  })
  return out
}

export function humanAction(action: string): string {
  const [object, verb] = action.split('.')
  const noun = object.replaceAll('_', ' ')
  return `${noun.charAt(0).toUpperCase()}${noun.slice(1)} ${(verb ?? '').replaceAll('_', ' ')}`.trim()
}

export const requestTypes: Record<string, string> = { allocation: 'Allocation', reservation: 'Reservation', release: 'Release' }

export const requestStatuses: Record<string, string> = {
  draft: 'Draft', submitted: 'Submitted', pending_approval: 'Pending approval', approved: 'Approved', rejected: 'Rejected',
  allocated: 'Allocated', reserved: 'Reserved', released: 'Released', cancelled: 'Cancelled', expired: 'Expired',
}

export type Tone = 'neutral' | 'accent' | 'ok' | 'warn' | 'danger'

export function requestTone(status: string): Tone {
  switch (status) {
    case 'pending_approval': return 'warn'
    case 'approved': return 'accent'
    case 'allocated': case 'reserved': case 'released': return 'ok'
    case 'rejected': return 'danger'
    default: return 'neutral'
  }
}

/** Short date for date-only values such as YYYY-MM-DD, read as calendar dates. */
export function formatDate(value: string): string {
  const [y, m, d] = value.slice(0, 10).split('-').map(Number)
  return new Date(Date.UTC(y, m - 1, d)).toLocaleDateString('en-GB', { dateStyle: 'medium', timeZone: 'UTC' })
}

/** Today as YYYY-MM-DD in UTC, plus a number of days. */
export function isoDate(addDays = 0): string {
  const d = new Date()
  d.setUTCDate(d.getUTCDate() + addDays)
  return d.toISOString().slice(0, 10)
}

/** Whole days from now until an instant; negative once it has passed. */
export function daysUntil(iso: string, now = Date.now()): number {
  return Math.ceil((new Date(iso).getTime() - now) / 86_400_000)
}

/**
 * A reservation runs to the end of its chosen day in UTC, stored as midnight
 * that follows it. Showing the day itself avoids an apparent off-by-one.
 */
export function formatEndOfDay(iso: string): string {
  return `End of ${formatDate(new Date(Date.parse(iso) - 1000).toISOString())}`
}
