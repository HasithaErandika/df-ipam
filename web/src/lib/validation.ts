/**
 * Checks that catch mistakes before a request is sent. The API checks everything
 * again and has the final word: these only make the answer quicker and clearer.
 * DF-IPAM Lite is IPv4 only.
 */
import { z } from 'zod'

export function isIPv4(v: string): boolean {
  const parts = v.split('.')
  return parts.length === 4 && parts.every((p) => /^\d{1,3}$/.test(p) && !(p.length > 1 && p.startsWith('0')) && Number(p) <= 255)
}

export function ipToInt(v: string): number {
  return v.split('.').reduce((n, p) => n * 256 + Number(p), 0)
}

export const intToIP = (n: number) => [24, 16, 8, 0].map((s) => Math.floor(n / 2 ** s) % 256).join('.')

/** A network written as its network address with a /16 to /30 prefix. */
export function cidrIssue(v: string): string | undefined {
  const [address, prefix, extra] = v.trim().split('/')
  if (extra !== undefined || !prefix || !/^\d{1,2}$/.test(prefix) || !isIPv4(address)) return 'Enter a network such as 192.168.4.0/24.'
  const bits = Number(prefix)
  if (bits < 16 || bits > 30) return 'Use a prefix from /16 to /30.'
  const size = 2 ** (32 - bits)
  const start = ipToInt(address)
  if (start % size !== 0) return `Use the network address: ${intToIP(start - (start % size))}/${bits}.`
  return undefined
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

/** An optional YYYY-MM-DD that is today or later. */
export const futureDate = (label: string) => z.string().refine((v) => v === '' || v >= new Date().toISOString().slice(0, 10), `${label} can't be in the past.`)

export const toInt = (v: string) => (v.trim() === '' ? null : Number(v))
