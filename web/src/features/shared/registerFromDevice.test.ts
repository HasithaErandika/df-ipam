import { describe, expect, it } from 'vitest'
import type { Device } from '../../lib/types'
import { fillFromDevice, ipTypeFromDevice, ipTypeFromIcon } from './registerFromDevice'

const device = { id: 'd1', mac: 'aa:bb:cc:dd:ee:ff', name: 'HR printer', owner: 'Nimal', department: 'HR', deviceType: 'printer', platform: 'hp' } as Device
const blank = { deviceId: '', ipType: 'other', assignedName: 'NPI3A5F21', platform: '', owner: '', department: '' }

describe('registering an approved device', () => {
  it('fills the type, name, platform, owner and department, and links the device', () => {
    expect(fillFromDevice(device, blank, () => false)).toEqual({
      deviceId: 'd1', ipType: 'printer', assignedName: 'HR printer', platform: 'hp', owner: 'Nimal', department: 'HR',
    })
  })
  it('keeps what the person typed', () => {
    const edited = (f: string) => f === 'owner' || f === 'ipType'
    const out = fillFromDevice(device, { ...blank, owner: 'Kamal', ipType: 'network' }, edited)
    expect(out.owner).toBeUndefined()
    expect(out.ipType).toBeUndefined()
    expect(out.deviceId).toBe('d1')
  })
  it('leaves fields the device has nothing for', () => {
    const out = fillFromDevice({ ...device, name: '', owner: '', department: '', platform: '', deviceType: 'phone' } as Device, blank, () => false)
    expect(out).toEqual({ deviceId: 'd1' })
  })
  it('maps device types and evidence to register types', () => {
    expect(ipTypeFromDevice('laptop')).toBe('user_pc')
    expect(ipTypeFromDevice('cctv')).toBe('cctv')
    expect(ipTypeFromDevice('phone')).toBeUndefined()
    expect(ipTypeFromIcon('camera')).toBe('cctv')
    expect(ipTypeFromIcon('huawei')).toBe('network')
    expect(ipTypeFromIcon('accesspoint')).toBe('access_point')
    expect(ipTypeFromIcon('android')).toBeUndefined()
  })
})
