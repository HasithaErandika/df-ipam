import { describe, expect, it } from 'vitest'
import { between, dateOrder, isCIDR, isCron, isEmail, isHttpsUrl, isIP, isIPv4, isIPv6, isMAC, issues, required } from './validation'

describe('addresses', () => {
  it('reads IPv4 strictly', () => {
    for (const v of ['10.0.0.1', '0.0.0.0', '255.255.255.255']) expect(isIPv4(v)).toBe(true)
    for (const v of ['256.0.0.1', '10.0.0', '10.0.0.1.1', '010.0.0.1', '10.0.0.a', '']) expect(isIPv4(v)).toBe(false)
  })
  it('reads IPv6', () => {
    for (const v of ['::', '::1', '2001:db8::1', 'fe80::1', '1:2:3:4:5:6:7:8', '::ffff:10.0.0.1']) expect(isIPv6(v)).toBe(true)
    for (const v of ['1:2:3:4:5:6:7', '1:2:3:4:5:6:7:8:9', ':::', '2001::db8::1', '12345::1', 'g::1', '10.0.0.1']) expect(isIPv6(v)).toBe(false)
  })
  it('reads CIDRs', () => {
    for (const v of ['10.0.0.0/24', '10.0.0.1/32', '0.0.0.0/0', '2001:db8::/32', '::/128']) expect(isCIDR(v)).toBe(true)
    for (const v of ['10.0.0.0', '10.0.0.0/33', '2001:db8::/129', '10.0.0.0/x', '10.0.0.0/24/1', '/24']) expect(isCIDR(v)).toBe(false)
  })
  it('accepts either family', () => {
    expect(isIP('10.0.0.1') && isIP('::1')).toBe(true)
    expect(isIP('10.0.0.0/24')).toBe(false)
  })
  it('reads MACs', () => {
    for (const v of ['00:11:22:33:44:55', '00-11-22-33-44-55', '0011.2233.4455']) expect(isMAC(v)).toBe(true)
    for (const v of ['00:11:22:33:44', '00:11:22:33:44:gg', '']) expect(isMAC(v)).toBe(false)
  })
})

describe('other fields', () => {
  it('needs https with no credentials', () => {
    expect(isHttpsUrl('https://fw.example.com:8443')).toBe(true)
    for (const v of ['http://fw.example.com', 'ftp://x', 'https://user:pw@fw.example.com', 'fw.example.com', '']) expect(isHttpsUrl(v)).toBe(false)
  })
  it('reads emails and cron', () => {
    expect(isEmail('a@example.com')).toBe(true)
    expect(isEmail('a@example')).toBe(false)
    expect(isCron('30 23 * * 1-5')).toBe(true)
    expect(isCron('30 23 * *')).toBe(false)
  })
  it('checks ranges and dates', () => {
    expect(between('5', 1, 10, 'X')).toBeUndefined()
    for (const v of ['0', '11', '', '1.5']) expect(between(v, 1, 10, 'X')).toBeDefined()
    expect(dateOrder('2026-02-01', '2026-01-01')).toBeDefined()
    expect(dateOrder('', '2026-01-01')).toBeUndefined()
  })
  it('keeps only failures', () => {
    expect(issues({ a: required('', 'A'), b: required('x', 'B') })).toEqual({ a: 'A is required.' })
  })
})
