import { describe, expect, it } from 'vitest'
import { deviceIcon, likelyPlatform, platformOf } from './platforms'

describe('platformOf', () => {
  it('prefers what a person recorded', () => {
    expect(platformOf('ubuntu', 'MSFT 5.0', 'DESKTOP-1')).toEqual({ platform: 'ubuntu', guessed: false })
  })

  it('guesses from the DHCP vendor class, then the hostname', () => {
    expect(platformOf('', 'MSFT 5.0', null)).toEqual({ platform: 'windows', guessed: true })
    expect(platformOf('', 'android-dhcp-14', null)?.platform).toBe('android')
    expect(platformOf('', '', 'Nimals-iPhone')?.platform).toBe('ios')
    expect(platformOf('', '', 'NPI3A5F21')?.platform).toBe('hp')
    expect(platformOf('', '', 'MacBook-Pro-7')?.platform).toBe('macos')
  })

  it('says nothing when there is no evidence', () => {
    expect(platformOf('', '', 'host-17')).toBeNull()
  })
})

describe('likelyPlatform', () => {
  it('never offers a maker or device kind', () => {
    expect(likelyPlatform({ vendor: 'Intel Corporate', hostname: 'ASUS-Q533MJ' })).toBeNull()
    expect(likelyPlatform({ vendor: 'Samsung · TV or media player' })).toBeNull()
  })

  it('reads the model, the evidence, then the TTL hint', () => {
    expect(likelyPlatform({ model: 'MacBookPro18,1' })).toBe('macos')
    expect(likelyPlatform({ model: 'iPhone14,2' })).toBe('ios')
    expect(likelyPlatform({ hostname: 'DESKTOP-7Q2' })).toBe('windows')
    expect(likelyPlatform({ vendor: 'Intel Corporate', osHint: 'Windows (TTL 128)' })).toBe('windows')
  })
})

describe('deviceIcon', () => {
  const ttl64 = 'Linux, Android or Apple (TTL 64)'
  it('never reads Android from a TTL of 64, which Linux and Apple share', () => {
    expect(deviceIcon({ osHint: ttl64 })).toBeNull()
  })
  it('shows a gateway as the router, whatever else is guessed', () => {
    expect(deviceIcon({ gateway: true, osHint: ttl64 })).toMatchObject({ code: 'router', guessed: false })
  })
  it('lets what a person recorded win', () => {
    expect(deviceIcon({ recorded: 'fortinet', gateway: true })).toMatchObject({ code: 'fortinet', guessed: false })
  })
  it('reads the device maker from its name, not the network card', () => {
    expect(deviceIcon({ hostname: 'ASUS-Q533MJ', osHint: ttl64 })?.code).toBe('asus')
  })
  it('uses specific evidence in order', () => {
    expect(deviceIcon({ dhcpClass: 'android-dhcp-14' })?.code).toBe('android')
    expect(deviceIcon({ openPorts: [22, 3389] })?.code).toBe('windows')
    expect(deviceIcon({ openPorts: [9100] })?.code).toBe('printer')
    expect(deviceIcon({ osHint: 'Windows (TTL 128)' })?.code).toBe('windows')
    expect(deviceIcon({ osHint: 'Network device or printer (TTL 255)' })?.code).toBe('network')
    expect(deviceIcon({ kind: 'Printer', hostname: 'DESKTOP-1' })?.code).toBe('printer')
    expect(deviceIcon({ model: 'MacBookPro18,1' })?.code).toBe('macos')
  })
})
