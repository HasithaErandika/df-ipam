import { describe, expect, it } from 'vitest'
import { formatRate, share } from './format'

describe('firewall rates', () => {
  it('reads bits a second in the right unit', () => {
    expect(formatRate(850_000)).toBe('850 kbps')
    expect(formatRate(12_400_000)).toBe('12.4 Mbps')
    expect(formatRate(1_200_000_000)).toBe('1.20 Gbps')
    expect(formatRate(null)).toBe('—')
  })
  it('measures use against the link bandwidth', () => {
    expect(share(50_000_000, 100)).toBe(50)
    expect(share(500_000_000, 100)).toBe(100)
    expect(share(1, null)).toBeNull()
  })
})
