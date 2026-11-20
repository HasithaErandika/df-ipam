import { describe, expect, it } from 'vitest'
import type { Finding } from '../../lib/types'
import { approveTarget, findingUser, registerPrefill } from './prefill'

const base = {
  id: 's1', ip: '10.1.2.30', mac: 'aa:bb:cc:dd:ee:ff', hostname: 'DF-LT-023', deviceName: null, deviceId: 'd1', segmentId: 'seg', segmentName: 'Floor 2',
  registerId: null, platform: '', vendor: '', nicVendor: '', maker: '', model: '', deviceKind: '', osHint: 'Windows (TTL 128)', source: 'arp', openPorts: [445],
  intuneUser: '', intuneUserName: '', intuneDevice: '', intuneSerial: '', intuneMaker: '', intuneModel: '', intuneOs: '',
  entraUser: '', entraUserName: '', entraDevice: '', vpnUser: '', workgroup: '', windowsDomain: '', osVersion: '',
} as unknown as Finding

describe('prefill from discovery', () => {
  it('takes the user from Intune, then Entra ID, then VPN', () => {
    expect(findingUser({ ...base, vpnUser: 'v@x', entraUser: 'e@x', intuneUser: 'i@x', intuneUserName: 'Intune Person' })).toBe('Intune Person')
    expect(findingUser({ ...base, vpnUser: 'v@x', entraUser: 'e@x' })).toBe('e@x')
    expect(findingUser({ ...base, vpnUser: 'v@x' })).toBe('v@x')
  })

  it('fills the register form with owner, platform and an evidence note', () => {
    const p = registerPrefill({ ...base, entraUser: 'nimal@dialogfinance.lk', entraUserName: 'Nimal P', intuneSerial: 'ABC123', intuneMaker: 'Dell', intuneModel: 'Latitude 5440', windowsDomain: 'DIALOGFINANCE' })
    expect(p).toMatchObject({ ip: '10.1.2.30', name: 'DF-LT-023', owner: 'Nimal P', platform: 'windows' })
    expect(p.notes).toBe('From discovery: Dell Latitude 5440, serial ABC123, domain DIALOGFINANCE.')
  })

  it('fills the approval with owner, platform and type', () => {
    expect(approveTarget({ ...base, intuneUser: 'kamal@dialogfinance.lk' })).toMatchObject({ name: 'DF-LT-023', owner: 'kamal@dialogfinance.lk', platform: 'windows', deviceType: 'pc' })
  })
})
