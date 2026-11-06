import { describe, expect, it } from 'vitest'
import { legacyTabUrl, tabPath } from './tabs'

describe('tabPath', () => {
  it('puts a tab under its page', () => expect(tabPath('/reconciliation', 'changes')).toBe('/reconciliation/changes'))
  it('keeps a tab named after its page at the page', () => expect(tabPath('/devices', 'devices')).toBe('/devices'))
})

describe('legacyTabUrl', () => {
  it('moves the tab into the path', () => expect(legacyTabUrl('/reconciliation', '?tab=changes')).toBe('/reconciliation/changes'))
  it('keeps other parameters', () => expect(legacyTabUrl('/requests', '?tab=mine&id=7')).toBe('/requests/mine?id=7'))
  it('leaves other addresses alone', () => expect(legacyTabUrl('/requests', '?id=7')).toBeNull())
})
