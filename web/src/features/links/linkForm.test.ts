import { describe, expect, it } from 'vitest'
import { bandwidthLine, formatMoney } from './labels'
import { clientIssues, emptyLink, toPayload } from './linkForm'

describe('network link formatting', () => {
  it('writes rupees as Rs. with two decimals, other currencies by code', () => {
    expect(formatMoney(25000)).toBe('Rs. 25,000.00')
    expect(formatMoney(1200.5, 'USD')).toBe('USD 1,200.50')
    expect(formatMoney(null)).toBe('—')
  })

  it('shows bandwidth in Mbps or Gbps', () => {
    expect(bandwidthLine(1000, 1000)).toBe('1 Gbps')
    expect(bandwidthLine(100, 20)).toBe('100 Mbps down / 20 Mbps up')
    expect(bandwidthLine(null, null)).toBe('')
  })
})

describe('the link form', () => {
  it('sends bandwidth in Mbps and remote subnets as a list', () => {
    const f = { ...emptyLink('Head Office'), name: 'ILL', linkType: 'ill', purpose: 'Internet', down: '1', downUnit: 'Gbps', up: '500', remoteSubnets: '10.0.0.0/16\n 172.16.0.0/24, ' }
    const p = toPayload(f)
    expect(p.downMbps).toBe(1000)
    expect(p.upMbps).toBe(500)
    expect(p.remoteSubnets).toEqual(['10.0.0.0/16', '172.16.0.0/24'])
  })

  it('drops the rental of a link with no separate cost', () => {
    expect(toPayload({ ...emptyLink(), noSeparateCost: true, monthlyRental: '5000' }).monthlyRental).toBeNull()
  })

  it('asks for the required fields and real numbers before sending', () => {
    const issues = clientIssues({ ...emptyLink(), vlan: 'ten' })
    expect(Object.keys(issues).sort()).toEqual(['linkType', 'name', 'purpose', 'sourceSite', 'vlan'])
  })
})
