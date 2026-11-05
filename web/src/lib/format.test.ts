import { describe, expect, it } from 'vitest'
import { formatAgo, daysUntil, formatCapacity, formatDate, formatEndOfDay, humanAction, pageWindow, requestTone, severity } from './format'

describe('formatCapacity', () => {
  it('shows IPv4 counts in full', () => expect(formatCapacity('65536', 4, 16)).toBe('65,536'))
  it('uses powers for large IPv6 ranges', () => {
    expect(formatCapacity('18446744073709551616', 6, 64)).toBe('2^64')
    expect(formatCapacity('340282366920938463463374607431768211456', 6, 0)).toBe('2^128')
  })
  it('stays exact beyond safe integers', () =>
    expect(formatCapacity('9007199254740993', 4, 0)).toBe('9,007,199,254,740,993'))
  it('shows small IPv6 ranges in full', () => expect(formatCapacity('2', 6, 127)).toBe('2'))
})

describe('severity', () => {
  it('uses the thresholds', () => {
    expect(severity(10, 80, 95)).toBe('normal')
    expect(severity(80, 80, 95)).toBe('warning')
    expect(severity(95, 80, 95)).toBe('critical')
  })
})

describe('pageWindow', () => {
  it('shows every page when there are few', () => expect(pageWindow(2, 3)).toEqual([1, 2, 3]))
  it('marks gaps around the current page', () => expect(pageWindow(10, 20)).toEqual([1, null, 9, 10, 11, null, 20]))
  it('fills a gap of one page instead of an ellipsis', () => expect(pageWindow(3, 20)).toEqual([1, 2, 3, 4, null, 20]))
  it('handles the ends', () => {
    expect(pageWindow(1, 20)).toEqual([1, 2, null, 20])
    expect(pageWindow(20, 20)).toEqual([1, null, 19, 20])
  })
  it('handles a single page', () => expect(pageWindow(1, 1)).toEqual([1]))
})

describe('humanAction', () => {
  it('reads naturally', () => expect(humanAction('routing_domain.create')).toBe('Routing domain create'))
})

describe('workflow formatting', () => {
  it('reads date-only values as calendar dates', () => expect(formatDate('2026-12-05')).toBe('5 Dec 2026'))
  it('shows a reservation end as the chosen day', () => expect(formatEndOfDay('2026-12-06T00:00:00Z')).toBe('End of 5 Dec 2026'))
  it('counts whole days left', () => {
    const now = Date.parse('2026-11-05T12:00:00Z')
    expect(daysUntil('2026-11-12T00:00:00Z', now)).toBe(7)
    expect(daysUntil('2026-11-05T11:00:00Z', now)).toBe(-0)
  })
  it('colours statuses by meaning', () => {
    expect(requestTone('pending_approval')).toBe('warn')
    expect(requestTone('rejected')).toBe('danger')
    expect(requestTone('allocated')).toBe('ok')
    expect(requestTone('cancelled')).toBe('neutral')
  })
})

describe('formatAgo', () => {
  const now = Date.parse('2026-11-05T12:00:00Z')
  it('says just now', () => expect(formatAgo('2026-11-05T11:59:30Z', now)).toBe('just now'))
  it('counts minutes', () => expect(formatAgo('2026-11-05T11:48:00Z', now)).toBe('12 minutes ago'))
  it('uses the singular', () => expect(formatAgo('2026-11-05T11:00:00Z', now)).toBe('1 hour ago'))
  it('counts days', () => expect(formatAgo('2026-11-02T12:00:00Z', now)).toBe('3 days ago'))
})
