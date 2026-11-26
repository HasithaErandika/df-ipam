import type { Tone } from '../../lib/labels'
import type { LinkSettings } from '../../lib/types'
import { formatDate, formatDateTime } from '../../lib/format'

export const linkStatuses: Record<string, string> = { active: 'Active', inactive: 'Inactive', pending: 'Pending', terminated: 'Terminated' }
export const linkStatusTone: Record<string, Tone> = { active: 'ok', inactive: 'neutral', pending: 'warn', terminated: 'danger' }
export const linkRoles: Record<string, string> = { primary: 'Primary', secondary: 'Secondary', backup: 'Backup', standalone: 'Standalone' }
export const criticalities: Record<string, string> = { critical: 'Critical', high: 'High', medium: 'Medium', low: 'Low' }
export const linkEnvironments: Record<string, string> = { production: 'Production', dr: 'DR', uat: 'UAT', other: 'Other' }
export const mediums: Record<string, string> = { fibre: 'Fibre', wireless: 'Wireless', copper: 'Copper', other: 'Other' }
export const routings: Record<string, string> = { static: 'Static', bgp: 'BGP', ospf: 'OSPF', other: 'Other' }
export const vpnTypes: Record<string, string> = { ipsec: 'IPsec', ssl: 'SSL', other: 'Other' }
export const relationKinds: Record<string, string> = { secondary: 'Secondary', backup: 'Backup', underlying: 'Underlying link' }
export const fileCategories: Record<string, string> = { contract: 'Contract', quotation: 'Provider quotation', diagram: 'Network diagram', configuration: 'Configuration (no secrets)', other: 'Other' }
/** Contracts and quotations need commercial access, to add and to open. */
export const commercialFiles = ['contract', 'quotation']

/** The link type's name, or its key when it is no longer listed. */
export const typeName = (s: LinkSettings | undefined, key: string) => s?.types.find((t) => t.key === key)?.label ?? key

export const isVpnType = (s: LinkSettings | undefined, key: string) => Boolean(s?.types.find((t) => t.key === key)?.vpn)

/** Rs. 25,000.00 for rupees, USD 1,200.00 for any other currency. */
export function formatMoney(amount: number | null | undefined, currency = 'LKR'): string {
  if (amount == null) return '—'
  const n = amount.toLocaleString('en-LK', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
  return currency === 'LKR' ? `Rs. ${n}` : `${currency} ${n}`
}

/** 1500 Mbps reads as 1.5 Gbps. */
export function formatBandwidth(mbps: number | null | undefined): string {
  if (mbps == null) return ''
  if (mbps >= 1000) return `${+(mbps / 1000).toFixed(3)} Gbps`
  return `${+mbps.toFixed(3)} Mbps`
}

export function bandwidthLine(down: number | null, up: number | null): string {
  if (down == null && up == null) return ''
  if (down === up) return formatBandwidth(down)
  return `${formatBandwidth(down) || '—'} down / ${formatBandwidth(up) || '—'} up`
}

/** A YYYY-MM-DD date as 15 Jan 2027, or a dash. */
export function formatDay(day: string | null | undefined): string {
  return day ? formatDate(day) : '—'
}

/** A time in Sri Lanka, as 15 Jan 2027, 14:05: the app-wide formatter. */
export const formatColombo = formatDateTime

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

/** Days until a contract ends, with the tone it deserves. */
export function expiryTone(days: number): Tone {
  if (days < 0) return 'danger'
  if (days <= 30) return 'danger'
  if (days <= 90) return 'warn'
  return 'neutral'
}

export function daysText(days: number): string {
  if (days < 0) return `Ended ${-days} day${days === -1 ? '' : 's'} ago`
  if (days === 0) return 'Ends today'
  return `${days} day${days === 1 ? '' : 's'} left`
}
