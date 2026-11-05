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
