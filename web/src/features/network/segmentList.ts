import type { Segment } from '../../lib/types'
import { maxScannable } from '../../lib/validation'

/** Cloud networks and VPN client pools are never swept from the building. */
export const notSwept = (kind: string) => kind === 'cloud' || kind === 'vpn'

/** A swept segment with more addresses than the scanner sweeps: it is never scanned. */
export const tooLargeToScan = (s: Segment) => !notSwept(s.kind) && s.usableSize > maxScannable

/** A segment that needs someone to look at it, with the reason. */
export function attention(s: Segment): string | null {
  if (tooLargeToScan(s)) return 'Too large to scan; split it'
  if (s.kind === 'dhcp' && !s.dhcpStart) return 'No DHCP pool set'
  if (!s.gateway && !s.detectedGateway && s.kind !== 'vpn') return 'No gateway'
  if (s.openAlerts > 0) return `${s.openAlerts} open alert${s.openAlerts === 1 ? '' : 's'}`
  return null
}

/** How full it is: the DHCP pool on DHCP networks, the register elsewhere. */
export function use(s: Segment): { pct: number; text: string } {
  if (s.kind === 'dhcp' && s.poolSize > 0) return { pct: (s.poolUsed * 100) / s.poolSize, text: `${s.poolUsed} of ${s.poolSize} in the pool` }
  const used = Math.max(s.registered, s.seen)
  return { pct: s.usableSize ? (used * 100) / s.usableSize : 0, text: `${s.registered} registered, ${s.seen} seen, of ${s.usableSize}` }
}

const ipNumber = (cidr: string) => cidr.split('/')[0].split('.').reduce((n, o) => n * 256 + Number(o), 0)

export const sorts: Record<string, { text: string; compare: (a: Segment, b: Segment) => number }> = {
  name: { text: 'Name', compare: (a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }) },
  network: { text: 'Address order', compare: (a, b) => ipNumber(a.network) - ipNumber(b.network) },
  use: { text: 'Most used', compare: (a, b) => use(b).pct - use(a).pct },
  vlan: { text: 'VLAN', compare: (a, b) => (a.vlan ?? 1e9) - (b.vlan ?? 1e9) },
}

export function matches(s: Segment, q: string): boolean {
  if (!q) return true
  const t = q.toLowerCase()
  return [s.name, s.network, s.gateway, s.description, s.environmentName, s.vlan !== null ? `vlan ${s.vlan}` : null]
    .some((v) => v?.toLowerCase().includes(t))
}

export const kindOrder = ['dhcp', 'server', 'cloud', 'vpn'] as const
export const kindTitle: Record<string, string> = { dhcp: 'DHCP networks (floors and Wi-Fi)', server: 'Server networks', cloud: 'Cloud networks', vpn: 'VPN client pools' }
