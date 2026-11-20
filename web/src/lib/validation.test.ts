import { describe, expect, it } from 'vitest'
import { cidrIssue, inNetwork, intToIP, ipToInt, ipv4, ipv4NetworkInfo, isIPv4, isMAC, networkOf, optionalInt, splitAddress } from './validation'

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
    expect(cidrIssue('10.0.0.0/8')).toBe('Use a prefix from /16 to /32.')
    expect(cidrIssue('122.255.36.14/32')).toBeUndefined()
    expect(ipv4NetworkInfo('122.255.36.14/32')?.usableAddresses).toBe(1)
    expect(ipv4NetworkInfo('172.16.5.6/31')?.firstUsable).toBe('172.16.5.6')
    expect(cidrIssue('nonsense')).toMatch(/Enter a network/)
  })

  it('reads addresses as network diagrams write them', () => {
    expect(splitAddress('10.10.40.253 /24')).toEqual({ ip: '10.10.40.253', bits: 24 })
    expect(splitAddress('122.255.4.226 255.255.255.248')).toEqual({ ip: '122.255.4.226', bits: 29 })
    expect(splitAddress('123.231.92.125')).toEqual({ ip: '123.231.92.125', bits: null })
    expect(splitAddress('172,22,60.145')).toBeNull()
    expect(splitAddress('10.0.0.1 255.0.255.0')).toBeNull()
    expect(networkOf('122.255.4.226', 29)).toBe('122.255.4.224/29')
    expect(networkOf('122.255.36.14', 32)).toBe('122.255.36.14/32')
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
