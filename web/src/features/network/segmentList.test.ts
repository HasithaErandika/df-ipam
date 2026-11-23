import { describe, expect, it } from 'vitest'
import { isPrivateNetwork, publicNotSwept } from './segmentList'

describe('isPrivateNetwork', () => {
  it('accepts the private ranges the scanner sweeps', () => {
    for (const n of ['10.0.0.0/8', '10.10.40.0/24', '172.16.5.4/30', '172.31.255.0/24', '192.168.14.0/24', '100.64.0.0/10', '100.127.1.0/24'])
      expect(isPrivateNetwork(n), n).toBe(true)
  })
  it('refuses public ranges and near misses', () => {
    for (const n of ['182.22.60.144/29', '20.10.40.0/24', '172.32.0.0/16', '172.15.0.0/16', '192.169.0.0/16', '100.128.0.0/16', '123.231.92.125/32', 'nonsense'])
      expect(isPrivateNetwork(n), n).toBe(false)
  })
  it('only flags swept types', () => {
    expect(publicNotSwept({ kind: 'server', network: '203.0.113.0/24' })).toBe(true)
    expect(publicNotSwept({ kind: 'cloud', network: '203.0.113.0/24' })).toBe(false)
    expect(publicNotSwept({ kind: 'dhcp', network: '192.168.2.0/24' })).toBe(false)
  })
})
