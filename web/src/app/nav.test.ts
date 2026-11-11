import { describe, expect, it } from 'vitest'
import { canOpen, navFor } from './nav'

// The fixed roles, from api/internal/auth/permissions.go.
const roles: Record<string, string[]> = {
  admin: ['inventory:view', 'inventory:edit', 'ip:request', 'ip:reserve', 'requests:approve', 'alerts:manage', 'reports:export', 'audit:view', 'users:manage', 'system:manage'],
  requester: ['inventory:view', 'ip:request', 'ip:reserve', 'reports:export'],
  viewer: ['inventory:view', 'reports:export'],
}

const labels = (role: string) => navFor(roles[role]).flatMap((g) => g.items.map((i) => i.label))
const network = ['Floors', 'Servers', 'Devices', 'Alerts']

describe('the menu follows the role', () => {
  it('gives Admins all nine screens', () => {
    expect(labels('admin')).toEqual(['Dashboard', ...network, 'IP Requests', 'Reservations', 'Reports', 'Settings'])
  })

  it('lets Requesters ask for IPs but not change settings', () => {
    expect(labels('requester')).toEqual(['Dashboard', ...network, 'IP Requests', 'Reservations', 'Reports'])
  })

  it('keeps Viewers to reading', () => {
    expect(labels('viewer')).toEqual(['Dashboard', ...network, 'Reservations', 'Reports'])
  })
})

describe('canOpen', () => {
  it('guards pages by address, tabs included', () => {
    expect(canOpen('/settings/users', roles.requester)).toBe(false)
    expect(canOpen('/settings/users', roles.admin)).toBe(true)
    expect(canOpen('/requests', roles.viewer)).toBe(false)
    expect(canOpen('/servers/production', roles.viewer)).toBe(true)
  })

  it('always opens the dashboard', () => {
    expect(canOpen('/', [])).toBe(true)
  })
})
