import { describe, expect, it } from 'vitest'
import { tabPath } from './tabs'

describe('tabPath', () => {
  it('puts a tab under its page', () => expect(tabPath('/servers', 'production')).toBe('/servers/production'))
  it('keeps a tab named after its page at the page', () => expect(tabPath('/devices', 'devices')).toBe('/devices'))
})
