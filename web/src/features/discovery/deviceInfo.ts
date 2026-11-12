import { makerFromName, makers } from '../../lib/platforms'

/** What the scanner knows about a device: from a finding, a device or a register entry. */
export interface Evidence {
  vendor: string; nicVendor: string; maker: string; model: string; deviceKind: string; osHint: string
  hostname: string | null; source: string; openPorts?: number[]
}

/** The service usually behind each port the scanner checks (api/internal/scanner/ports.go). */
export const portServices: Record<number, string> = {
  21: 'FTP', 22: 'SSH', 23: 'Telnet', 25: 'SMTP', 53: 'DNS', 80: 'HTTP', 110: 'POP3', 135: 'Windows RPC', 139: 'NetBIOS', 143: 'IMAP',
  443: 'HTTPS', 445: 'File sharing (SMB)', 515: 'Printer (LPD)', 631: 'Printer (IPP)', 1433: 'SQL Server', 1521: 'Oracle', 3306: 'MySQL',
  3389: 'Remote Desktop', 5432: 'PostgreSQL', 5900: 'VNC', 5985: 'WinRM', 6379: 'Redis', 8080: 'HTTP (alt)', 8443: 'HTTPS (alt)',
  9100: 'Printer (raw)', 27017: 'MongoDB',
}

export const serviceName = (port: number) => portServices[port] ?? `Port ${port}`

/** What open ports say: Windows-only services, or printing. */
function fromPorts(ports: number[] = []): { system: string; kind: string } {
  const has = (...p: number[]) => p.some((x) => ports.includes(x))
  return {
    system: has(3389, 135, 5985) ? 'Windows (Windows services open)' : '',
    kind: has(9100, 515, 631) ? 'Printer' : '',
  }
}

/**
 * The maker of the network card, from the MAC's first half. It names the card,
 * not the device: an ASUS laptop with an Intel Wi-Fi card shows Intel. Rows from
 * before the card was recorded on its own still hold it in vendor.
 */
export function networkCard(r: Evidence): string {
  if (r.nicVendor) return r.nicVendor
  return r.source !== 'dhcp' && !r.maker && r.vendor && !r.vendor.includes(' · ') ? r.vendor : ''
}

/** What the device says it is, best evidence first, with where each part came from. */
export function deviceFacts(r: Evidence): { maker: string; makerFrom: string; model: string; kind: string; system: string; dhcpClass: string } {
  const named = makerFromName(r.hostname)
  const ports = fromPorts(r.openPorts)
  const maker = r.maker || (named ? makers[named] : '')
  return {
    maker,
    makerFrom: r.maker ? 'It said so itself (UPnP)' : named ? 'From its name' : '',
    model: r.model,
    kind: r.deviceKind || ports.kind,
    system: ports.system || r.osHint,
    dhcpClass: r.source === 'dhcp' ? r.vendor : '',
  }
}

/** One line for the table: maker, model and kind, or the system hint when that is all there is. */
export function deviceLine(r: Evidence): string {
  const f = deviceFacts(r)
  const parts = [f.maker, f.model, f.kind].filter(Boolean)
  if (parts.length === 0 && f.system) parts.push(`Likely ${f.system.replace(/ \(TTL \d+\)$/, '')}`)
  if (f.dhcpClass) parts.push(f.dhcpClass)
  return [...new Set(parts)].join(' · ')
}
