import { describe, expect, it } from 'vitest'
import { attentionRow, isDhcpClient, needsRegister } from './findingColumns'

const base = { registerId: null, isGateway: false, isStatic: false, segmentKind: 'dhcp', deviceStatus: 'approved', mac: '00:11:22:33:44:55', isPrivateMac: false, openAlerts: 0 }

describe('Discovery attention colours', () => {
  it('leaves an approved DHCP client plain: its address changes', () => {
    expect(needsRegister(base)).toBe(false)
    expect(attentionRow(base)).toBeUndefined()
  })
  it('marks an unknown device and a missing register entry orange', () => {
    expect(attentionRow({ ...base, deviceStatus: null })).toContain('warn')
    expect(attentionRow({ ...base, isStatic: true })).toContain('warn')
    expect(attentionRow({ ...base, segmentKind: 'server' })).toContain('warn')
  })
  it('marks open alerts and rejected devices red', () => {
    expect(attentionRow({ ...base, openAlerts: 1 })).toContain('danger')
    expect(attentionRow({ ...base, deviceStatus: 'rejected' })).toContain('danger')
  })
  it('never marks a gateway or a registered address for registering', () => {
    expect(needsRegister({ ...base, isGateway: true, segmentKind: 'server' })).toBe(false)
    expect(needsRegister({ ...base, registerId: 'x', segmentKind: 'server' })).toBe(false)
  })
  it('never asks to register a DHCP pool address: the device is approved instead', () => {
    expect(needsRegister({ ...base, deviceStatus: null })).toBe(false)
    expect(needsRegister({ ...base, deviceStatus: 'pending' })).toBe(false)
    expect(attentionRow({ ...base, deviceStatus: 'pending' })).toContain('warn')
    expect(isDhcpClient(base)).toBe(true)
    expect(isDhcpClient({ ...base, isStatic: true })).toBe(false)
    expect(isDhcpClient({ ...base, segmentKind: 'server' })).toBe(false)
    expect(isDhcpClient({ isStatic: false, inPool: true })).toBe(true)
  })
})
