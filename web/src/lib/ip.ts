/**
 * Address arithmetic with BigInt, so IPv6 offsets stay exact. Nothing here ever
 * lists a range: it only converts between an address and its position.
 */

export interface Parsed { family: 4 | 6; value: bigint }

export function parseIPv4(s: string): bigint | null {
  const parts = s.split('.')
  if (parts.length !== 4) return null
  let v = 0n
  for (const p of parts) {
    if (!/^\d{1,3}$/.test(p) || Number(p) > 255 || (p.length > 1 && p.startsWith('0'))) return null
    v = (v << 8n) | BigInt(p)
  }
  return v
}

export function parseIPv6(s: string): bigint | null {
  if (!s.includes(':') || s.includes(':::')) return null
  let text = s
  let tail: bigint[] = []
  const lastColon = text.lastIndexOf(':')
  if (text.slice(lastColon + 1).includes('.')) {
    const v4 = parseIPv4(text.slice(lastColon + 1))
    if (v4 === null) return null
    tail = [v4 >> 16n, v4 & 0xffffn]
    text = text.slice(0, lastColon + 1) + '0'
  }
  const halves = text.split('::')
  if (halves.length > 2) return null
  const group = (h: string) => (h === '' ? [] : h.split(':'))
  const head = group(halves[0])
  const rest = halves.length === 2 ? group(halves[1]) : []
  const words = [...head, ...rest]
  if (words.some((w) => !/^[0-9a-f]{1,4}$/i.test(w))) return null
  let values = words.map((w) => BigInt(`0x${w}`))
  if (tail.length) values = [...values.slice(0, -1), ...tail]
  const headCount = head.length
  const restCount = values.length - headCount
  const missing = 8 - values.length
  if (halves.length === 1 ? missing !== 0 : missing < 1) return null
  const full = [...values.slice(0, headCount), ...Array(halves.length === 2 ? missing : 0).fill(0n), ...values.slice(headCount, headCount + restCount)]
  return full.reduce((acc, w) => (acc << 16n) | w, 0n)
}

export function parseAddress(s: string): Parsed | null {
  const t = s.trim()
  const v4 = parseIPv4(t)
  if (v4 !== null) return { family: 4, value: v4 }
  const v6 = parseIPv6(t)
  return v6 === null ? null : { family: 6, value: v6 }
}

export function formatAddress(family: 4 | 6, value: bigint): string {
  if (family === 4) return [24n, 16n, 8n, 0n].map((s) => String((value >> s) & 0xffn)).join('.')
  const words = Array.from({ length: 8 }, (_, i) => Number((value >> BigInt(112 - i * 16)) & 0xffffn))
  // Compress the longest run of zero words (RFC 5952).
  let best = -1, bestLen = 0
  for (let i = 0; i < 8;) {
    if (words[i] !== 0) { i++; continue }
    let j = i
    while (j < 8 && words[j] === 0) j++
    if (j - i > bestLen && j - i > 1) { best = i; bestLen = j - i }
    i = j
  }
  const hex = words.map((w) => w.toString(16))
  if (best < 0) return hex.join(':')
  return `${hex.slice(0, best).join(':')}::${hex.slice(best + bestLen).join(':')}`
}

export interface Cidr { family: 4 | 6; network: bigint; bits: number; size: bigint }

export function parseCidr(s: string): Cidr | null {
  const [addr, len] = s.trim().split('/')
  const a = parseAddress(addr ?? '')
  const bits = Number(len)
  if (!a || !/^\d+$/.test(len ?? '')) return null
  const width = a.family === 4 ? 32 : 128
  if (bits > width) return null
  const size = 1n << BigInt(width - bits)
  return { family: a.family, network: a.value - (a.value % size), bits, size }
}

/** Position of an address inside a CIDR, or null when it falls outside. */
export function offsetIn(cidr: string, address: string): bigint | null {
  const c = parseCidr(cidr)
  const a = parseAddress(address)
  if (!c || !a || a.family !== c.family) return null
  const off = a.value - c.network
  return off >= 0n && off < c.size ? off : null
}
