/**
 * Checks that catch mistakes before a request is sent. The API checks everything
 * again and has the final word: these only make the answer quicker and clearer.
 * Each check returns a message, or undefined when the value is fine.
 */
export type Issue = string | undefined

export function isIPv4(v: string): boolean {
  const parts = v.split('.')
  return parts.length === 4 && parts.every((p) => /^\d{1,3}$/.test(p) && !(p.length > 1 && p.startsWith('0')) && Number(p) <= 255)
}

export function isIPv6(v: string): boolean {
  if (!/^[0-9a-fA-F:.]+$/.test(v) || !v.includes(':') || v.includes(':::')) return false
  const halves = v.split('::')
  if (halves.length > 2) return false
  const groups = halves.flatMap((h) => (h === '' ? [] : h.split(':')))
  let count = groups.length
  const last = groups[groups.length - 1]
  if (last?.includes('.')) {
    if (!isIPv4(last)) return false
    count += 1 // an embedded IPv4 address fills two groups
    groups.pop()
  }
  if (!groups.every((g) => /^[0-9a-fA-F]{1,4}$/.test(g))) return false
  return halves.length === 2 ? count < 8 : count === 8
}

export const isIP = (v: string) => isIPv4(v) || isIPv6(v)

export function isCIDR(v: string): boolean {
  const [address, prefix, extra] = v.split('/')
  if (extra !== undefined || !prefix || !/^\d{1,3}$/.test(prefix)) return false
  const n = Number(prefix)
  return isIPv4(address) ? n <= 32 : isIPv6(address) && n <= 128
}

export const isMAC = (v: string) => /^([0-9a-fA-F]{2}[:-]){5}[0-9a-fA-F]{2}$/.test(v) || /^([0-9a-fA-F]{4}\.){2}[0-9a-fA-F]{4}$/.test(v)

export const isEmail = (v: string) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v)

export function isHttpsUrl(v: string): boolean {
  try {
    const u = new URL(v)
    return u.protocol === 'https:' && u.hostname !== '' && u.username === '' && u.password === ''
  } catch {
    return false
  }
}

/** Five fields: minute hour day month weekday. The API checks the ranges. */
export const isCron = (v: string) => /^[\d*/,-]+( [\d*/,-]+){4}$/.test(v.trim())

export const isUUID = (v: string) => /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(v.trim())

export const required = (v: string | null | undefined, label: string): Issue => (v?.trim() ? undefined : `${label} is required.`)

export const choose = (v: string | null | undefined, label: string): Issue => (v ? undefined : `Choose ${label}.`)

export const optionalIP = (v: string): Issue => (v.trim() === '' || isIP(v.trim()) ? undefined : 'Enter a valid IPv4 or IPv6 address.')

export const ip = (v: string): Issue => (isIP(v.trim()) ? undefined : 'Enter a valid IPv4 or IPv6 address.')

export const cidr = (v: string): Issue => (isCIDR(v.trim()) ? undefined : 'Enter a network such as 10.20.30.0/24.')

export const optionalMAC = (v: string): Issue => (v.trim() === '' || isMAC(v.trim()) ? undefined : 'Enter a MAC address such as 00:11:22:33:44:55.')

export const email = (v: string): Issue => (isEmail(v.trim()) ? undefined : 'Enter a valid email address.')

export function between(v: string | number, min: number, max: number, label: string): Issue {
  const n = Number(v)
  return v === '' || !Number.isInteger(n) || n < min || n > max ? `${label} must be a whole number from ${min} to ${max}.` : undefined
}

export const dateOrder = (from: string, to: string): Issue => (from && to && to < from ? 'The end is before the start.' : undefined)

export const maxLength = (v: string, max: number, label: string): Issue => (v.length > max ? `${label} can be at most ${max} characters.` : undefined)

/** Drops the fields that passed, so an empty object means the form is valid. */
export function issues(checks: Record<string, Issue>): Record<string, string> {
  return Object.fromEntries(Object.entries(checks).filter((e): e is [string, string] => Boolean(e[1])))
}
