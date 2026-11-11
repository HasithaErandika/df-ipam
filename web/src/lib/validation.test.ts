import { describe, expect, it } from 'vitest'
import { cidrIssue, inNetwork, intToIP, ipToInt, ipv4, isIPv4, isMAC, optionalInt } from './validation'

describe('IPv4', () => {
  it('accepts dotted quads only', () => {
    expect(isIPv4('192.168.1.10')).toBe(true)
    expect(isIPv4('192.168.1.256')).toBe(false)
    expect(isIPv4('192.168.01.1')).toBe(false)
    expect(isIPv4('fd00::1')).toBe(false)
    expect(ipv4.safeParse(' 10.0.0.1 ').success).toBe(true)
  })

  it('round-trips through numbers', () => {
    expect(intToIP(ipToInt('172.16.10.5'))).toBe('172.16.10.5')
  })

  it('checks networks like the API does', () => {
    expect(cidrIssue('192.168.4.0/24')).toBeUndefined()
    expect(cidrIssue('192.168.4.1/24')).toBe('Use the network address: 192.168.4.0/24.')
    expect(cidrIssue('10.0.0.0/8')).toBe('Use a prefix from /16 to /30.')
    expect(cidrIssue('nonsense')).toMatch(/Enter a network/)
  })

  it('knows what is inside a network', () => {
    expect(inNetwork('192.168.4.200', '192.168.4.0/24')).toBe(true)
    expect(inNetwork('192.168.5.1', '192.168.4.0/24')).toBe(false)
  })
})

describe('other fields', () => {
  it('reads MAC addresses in common forms', () => {
    expect(isMAC('00:11:22:33:44:55')).toBe(true)
    expect(isMAC('00-11-22-33-44-55')).toBe(true)
    expect(isMAC('0011.2233.4455')).toBe(true)
    expect(isMAC('00:11:22:33:44')).toBe(false)
  })

  it('keeps optional numbers in range', () => {
    const vlan = optionalInt('VLAN', 1, 4094)
    expect(vlan.safeParse('').success).toBe(true)
    expect(vlan.safeParse('40').success).toBe(true)
    expect(vlan.safeParse('5000').success).toBe(false)
    expect(vlan.safeParse('4.5').success).toBe(false)
  })
})
