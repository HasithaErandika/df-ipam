/**
 * Checks that catch mistakes before a request is sent. The API checks everything
 * again and has the final word: these only make the answer quicker and clearer.
 * DF-IPAM Lite is IPv4 only.
 */
import { z } from 'zod'
import { isoDate } from './format'

export function isIPv4(v: string): boolean {
  const parts = v.split('.')
  return parts.length === 4 && parts.every((p) => /^\d{1,3}$/.test(p) && !(p.length > 1 && p.startsWith('0')) && Number(p) <= 255)
}

export function ipToInt(v: string): number {
  return v.split('.').reduce((n, p) => n * 256 + Number(p), 0)
}

export const intToIP = (n: number) => [24, 16, 8, 0].map((s) => Math.floor(n / 2 ** s) % 256).join('.')

export interface IPv4NetworkInfo {
  network: string
  prefixLength: number
  mask: string
  broadcast: string
  firstUsable: string
  lastUsable: string
  totalAddresses: number
  usableAddresses: number
}

/** Subnet facts for the /16–/32 networks accepted by segment setup. /31 and /32 have no network or broadcast address. */
export function ipv4NetworkInfo(cidr: string): IPv4NetworkInfo | null {
  if (cidrIssue(cidr)) return null
  const [address, prefixText] = cidr.trim().split('/')
  const prefixLength = Number(prefixText)
  const size = 2 ** (32 - prefixLength)
  const network = ipToInt(address)
  const broadcast = network + size - 1
  return {
    network: intToIP(network), prefixLength,
    mask: intToIP(0xffffffff - size + 1), broadcast: intToIP(broadcast),
    firstUsable: intToIP(size <= 2 ? network : network + 1), lastUsable: intToIP(size <= 2 ? broadcast : broadcast - 1),
    totalAddresses: size, usableAddresses: size <= 2 ? size : size - 2,
  }
}

/** A network written as its network address with a /16 to /32 prefix. */
export function cidrIssue(v: string): string | undefined {
  const [address, prefix, extra] = v.trim().split('/')
  if (extra !== undefined || !prefix || !/^\d{1,2}$/.test(prefix) || !isIPv4(address)) return 'Enter a network such as 192.168.4.0/24.'
  const bits = Number(prefix)
  if (bits < 16 || bits > 32) return 'Use a prefix from /16 to /32.'
  const size = 2 ** (32 - bits)
  const start = ipToInt(address)
  if (start % size !== 0) return `Use the network address: ${intToIP(start - (start % size))}/${bits}.`
  return undefined
}

/** A contiguous dotted mask (255.255.255.248) as a prefix length, or null. */
export function maskBits(mask: string): number | null {
  if (!isIPv4(mask)) return null
  const bin = ipToInt(mask).toString(2).padStart(32, '0')
  return /^1*0*$/.test(bin) ? bin.indexOf('0') === -1 ? 32 : bin.indexOf('0') : null
}

/**
 * Reads an address the way network diagrams write it: "10.10.40.253",
 * "10.10.40.253 /24" or "122.255.4.226 255.255.255.248". bits is null when no
 * prefix or mask is given; the result is null when it can't be read.
 */
export function splitAddress(v: string): { ip: string; bits: number | null } | null {
  const t = v.trim()
  const slash = t.indexOf('/')
  const parts = t.split(/\s+/)
  const [ip, prefix] = slash >= 0 ? [t.slice(0, slash).trim(), t.slice(slash + 1).trim()] : parts.length === 2 ? parts : [t, '']
  if (!isIPv4(ip)) return null
  if (prefix === '') return slash >= 0 ? null : { ip, bits: null }
  const bits = /^\d{1,2}$/.test(prefix) ? Number(prefix) : maskBits(prefix)
  return bits == null || bits > 32 ? null : { ip, bits }
}

/** The network an address sits in, such as 10.10.40.0/24 for 10.10.40.253 and 24. */
export function networkOf(ip: string, bits: number): string {
  const size = 2 ** (32 - bits)
  const n = ipToInt(ip)
  return `${intToIP(n - (n % size))}/${bits}`
}

/** The scanner skips segments with more usable addresses than this (a /20). */
export const maxScannable = 4096

/** Whether an address is a usable host in the network: not its network or broadcast address. */
export function isHostAddress(ip: string, cidr: string): boolean {
  const info = ipv4NetworkInfo(cidr)
  return Boolean(info && isIPv4(ip) && ipToInt(ip) >= ipToInt(info.firstUsable) && ipToInt(ip) <= ipToInt(info.lastUsable))
}

/** Whether an IPv4 address is inside a CIDR network. */
export function inNetwork(ip: string, cidr: string): boolean {
  const [address, prefix] = cidr.split('/')
  if (!isIPv4(ip) || !isIPv4(address)) return false
  const size = 2 ** (32 - Number(prefix))
  const start = ipToInt(address)
  return ipToInt(ip) >= start && ipToInt(ip) < start + size
}

export const isMAC = (v: string) => /^([0-9a-fA-F]{2}[:-]){5}[0-9a-fA-F]{2}$/.test(v) || /^([0-9a-fA-F]{4}\.){2}[0-9a-fA-F]{4}$/.test(v)

export const isEmail = (v: string) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v)

// Zod pieces shared by forms. Messages match the API's wording.

export const text = (label: string, max: number) => z.string().trim().max(max, `${label} can be at most ${max} characters.`)

export const requiredText = (label: string, max: number) => text(label, max).min(1, `${label} is required.`)

export const ipv4 = z.string().trim().refine(isIPv4, 'Enter a valid IPv4 address such as 192.168.1.10.')

export const optionalIPv4 = z.string().trim().refine((v) => v === '' || isIPv4(v), 'Enter a valid IPv4 address such as 192.168.1.10.')

export const optionalMAC = z.string().trim().refine((v) => v === '' || isMAC(v), 'Enter a MAC address such as 00:11:22:33:44:55.')

export const choose = (what: string) => z.string().min(1, `Choose ${what}.`)

/** A whole number from an input, or null when left empty. */
export const optionalInt = (label: string, min: number, max: number) => z.string().trim()
  .refine((v) => v === '' || (/^-?\d+$/.test(v) && Number(v) >= min && Number(v) <= max), `${label} must be a whole number from ${min} to ${max}.`)

/** An optional YYYY-MM-DD that is today or later, by the person's own calendar. */
export const futureDate = (label: string) => z.string().refine((v) => v === '' || v >= isoDate(), `${label} can't be in the past.`)

export const toInt = (v: string) => (v.trim() === '' ? null : Number(v))
