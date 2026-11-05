import { describe, expect, it } from 'vitest'
import { formatAddress, offsetIn, parseAddress, parseCidr } from './ip'

describe('parseAddress', () => {
  it('reads IPv4', () => expect(parseAddress('10.0.1.2')).toEqual({ family: 4, value: 0x0a000102n }))
  it('rejects bad IPv4', () => {
    for (const bad of ['10.0.1', '10.0.1.256', '10.0.01.2', 'a.b.c.d']) expect(parseAddress(bad)).toBeNull()
  })
  it('reads compressed IPv6', () => expect(parseAddress('2001:db8::1')?.value).toBe(0x20010db8000000000000000000000001n))
  it('reads :: and full forms', () => {
    expect(parseAddress('::')?.value).toBe(0n)
    expect(parseAddress('ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff')?.value).toBe((1n << 128n) - 1n)
  })
  it('reads embedded IPv4', () => expect(parseAddress('::ffff:10.0.0.1')?.value).toBe(0xffff0a000001n))
  it('rejects bad IPv6', () => {
    for (const bad of ['1::2::3', '1:2:3:4:5:6:7:8:9', '12345::', 'g::1', '1:2:3:4:5:6:7']) expect(parseAddress(bad)).toBeNull()
  })
})

describe('formatAddress', () => {
  it('round-trips IPv4', () => expect(formatAddress(4, 0x0a000102n)).toBe('10.0.1.2'))
  it('compresses the longest zero run', () => {
    expect(formatAddress(6, parseAddress('2001:db8:0:0:1:0:0:1')!.value)).toBe('2001:db8::1:0:0:1')
    expect(formatAddress(6, 0n)).toBe('::')
    expect(formatAddress(6, 1n)).toBe('::1')
  })
})

describe('offsetIn', () => {
  it('finds a position in IPv4', () => expect(offsetIn('10.0.0.0/24', '10.0.0.200')).toBe(200n))
  it('finds a position deep in IPv6 without listing', () =>
    expect(offsetIn('2001:db8::/64', '2001:db8::1:0:0:5')).toBe((1n << 48n) + 5n))
  it('refuses outside addresses and other families', () => {
    expect(offsetIn('10.0.0.0/24', '10.0.1.1')).toBeNull()
    expect(offsetIn('10.0.0.0/24', '::1')).toBeNull()
  })
  it('masks host bits in a CIDR', () => expect(parseCidr('10.0.0.9/30')?.network).toBe(0x0a000008n))
})
