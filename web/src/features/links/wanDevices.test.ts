import { describe, expect, it } from 'vitest'
import { linkIps } from './DevicesField'
import { emptyLink, toPayload } from './linkForm'
import { wanRowIssue, wanRowOf, wanRowValue } from './MoreWanField'

describe('more WAN addresses', () => {
  it('keeps each address with its subnet', () => {
    expect(wanRowOf('122.255.35.1/32')).toEqual({ ip: '122.255.35.1', subnet: '122.255.35.1/32' })
    expect(wanRowOf('10.10.40.253/24')).toEqual({ ip: '10.10.40.253', subnet: '10.10.40.0/24' })
    expect(wanRowValue({ ip: '122.255.35.1', subnet: '' })).toBe('122.255.35.1/32')
    expect(wanRowValue({ ip: '10.10.40.9', subnet: '10.10.40.0/24' })).toBe('10.10.40.9/24')
    expect(wanRowIssue({ ip: '10.10.41.9', subnet: '10.10.40.0/24' })).toMatch(/isn't in/)
    expect(wanRowIssue({ ip: '172,22,60.145', subnet: '' })).toMatch(/isn't an IPv4/)
  })

  it('sends the rows and asks the scan about every WAN address and the gateway', () => {
    const f = { ...emptyLink('Head Office'), wanIp: '122.255.36.14', gatewayIp: '122.255.36.13', extraWan: [{ ip: '122.255.35.1', subnet: '' }, { ip: '', subnet: '' }] }
    expect(toPayload(f).extraWanIps).toEqual(['122.255.35.1/32'])
    expect(linkIps(f)).toEqual(['122.255.35.1', '122.255.36.13', '122.255.36.14'])
  })
})
