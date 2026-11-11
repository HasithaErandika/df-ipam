import { describe, expect, it } from 'vitest'
import { platformOf } from './platforms'

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
