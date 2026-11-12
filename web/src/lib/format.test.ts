import { describe, expect, it } from 'vitest'
import { daysUntil, endOfDayDate, formatAgo, formatDate, level, pageWindow, segmentCaption } from './format'

describe('level', () => {
  it('grades a percentage', () => {
    expect(level(50, 75, 90)).toBe('normal')
    expect(level(80, 75, 90)).toBe('warning')
    expect(level(95, 75, 90)).toBe('critical')
  })
})

describe('pageWindow', () => {
  it('shows the ends and the neighbours of the current page', () => {
    expect(pageWindow(1, 1)).toEqual([1])
    expect(pageWindow(5, 10)).toEqual([1, null, 4, 5, 6, null, 10])
    expect(pageWindow(2, 4)).toEqual([1, 2, 3, 4])
  })
})

describe('dates', () => {
  const now = Date.parse('2026-11-11T12:00:00Z')

  it('says how long ago', () => {
    expect(formatAgo('2026-11-11T11:59:40Z', now)).toBe('just now')
    expect(formatAgo('2026-11-11T11:48:00Z', now)).toBe('12 min ago')
    expect(formatAgo('2026-11-11T09:00:00Z', now)).toBe('3 h ago')
    expect(formatAgo('2026-11-08T12:00:00Z', now)).toBe('3 days ago')
  })

  it('reads calendar dates without shifting the day', () => {
    expect(formatDate('2026-03-01')).toBe('1 Mar 2026')
  })

  it('gives back the day a reservation was set to end', () => {
    expect(endOfDayDate('2026-12-01T00:00:00Z')).toBe('2026-11-30')
    expect(daysUntil('2026-11-14T12:00:00Z', now)).toBe(3)
  })
})

describe('segmentCaption', () => {
  it('joins the parts that are set', () => {
    expect(segmentCaption({ floor: 4, vlan: 40, network: '192.168.4.0/24' })).toBe('Floor 4 · VLAN 40 · 192.168.4.0/24')
    expect(segmentCaption({ floor: null, vlan: null, network: '172.16.10.0/24' })).toBe('172.16.10.0/24')
  })
})
