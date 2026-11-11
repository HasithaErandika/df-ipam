import { describe, expect, it } from 'vitest'
import { likelyPlatform, platformOf } from './platforms'

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
