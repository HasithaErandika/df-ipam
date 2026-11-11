import { describe, expect, it } from 'vitest'
import { listChanges } from './changes'

describe('listChanges', () => {
  it('lists only labelled fields that changed', () => {
    const before = { name: 'Floor 1', gateway: '', alert: false, secret: 'a' }
    const after = { name: 'Floor 1', gateway: '192.168.1.1', alert: true, secret: 'b' }
    expect(listChanges(before, after, { name: 'Name', gateway: 'Gateway', alert: 'Alerts' }))
      .toEqual(['Gateway: none → 192.168.1.1', 'Alerts: off → on'])
  })

  it('says nothing when nothing changed', () => {
    expect(listChanges({ a: 1 }, { a: 1 }, { a: 'A' })).toEqual([])
  })
})
