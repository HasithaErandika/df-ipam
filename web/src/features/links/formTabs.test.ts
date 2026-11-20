import { describe, expect, it } from 'vitest'
import { tabOf, tabsWithErrors } from './formTabs'

describe('link dialog tabs', () => {
  it('sends each problem to its tab, server names included', () => {
    expect(tabOf('name')).toBe('basic')
    expect(tabOf('downMbps')).toBe('technical')
    expect(tabOf('remoteSubnets')).toBe('vpn')
    expect(tabOf('contractEnd')).toBe('commercial')
    expect(tabOf('files')).toBe('notes')
  })

  it('marks only the tabs shown', () => {
    expect(tabsWithErrors({ vlan: 'x', remotePeer: 'y', name: 'z' }, ['basic', 'technical', 'related', 'notes'])).toEqual(['basic', 'technical'])
  })
})
