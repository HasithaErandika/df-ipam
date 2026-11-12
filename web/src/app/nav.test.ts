import { describe, expect, it } from 'vitest'
import { allPermissions } from '../lib/permissions'
import { canOpen, navFor } from './nav'

// The starting roles, from api/migrations/000012_lite_roles.up.sql.
const roles: Record<string, string[]> = {
  admin: allPermissions.map((p) => p.key),
  network_engineer: ['inventory:view', 'inventory:edit', 'network:manage', 'discovery:scan', 'ip:request', 'ip:reserve', 'requests:approve', 'alerts:manage', 'reports:export'],
  security_auditor: ['inventory:view', 'audit:view', 'reports:export'],
  requester: ['inventory:view', 'ip:request', 'ip:reserve', 'reports:export'],
  viewer: ['inventory:view', 'reports:export'],
}

const labels = (role: string) => navFor(roles[role]).flatMap((g) => g.items.map((i) => i.label))
const network = ['Discovery', 'Servers', 'Devices', 'Alerts']

describe('the menu follows the role', () => {
  it('gives administrators every screen, with administration split in three', () => {
    expect(labels('admin')).toEqual(['Dashboard', ...network, 'IP Requests', 'Reservations', 'Reports', 'Audit log', 'Network setup', 'Users and roles', 'System'])
  })

  it('gives network engineers network setup, but not people or system settings', () => {
    expect(labels('network_engineer')).toEqual(['Dashboard', ...network, 'IP Requests', 'Reservations', 'Reports', 'Network setup'])
  })

  it('gives auditors the audit log', () => {
    expect(labels('security_auditor')).toEqual(['Dashboard', ...network, 'Reservations', 'Reports', 'Audit log'])
  })

  it('keeps viewers to reading', () => {
    expect(labels('viewer')).toEqual(['Dashboard', ...network, 'Reservations', 'Reports'])
  })

  it('never lists drill-down pages', () => {
    expect(labels('admin')).not.toContain('Segment')
  })
})

describe('canOpen', () => {
  it('guards pages by address, tabs included', () => {
    expect(canOpen('/access/roles', roles.network_engineer)).toBe(false)
    expect(canOpen('/access/roles', roles.admin)).toBe(true)
    expect(canOpen('/network-setup/departments', roles.network_engineer)).toBe(true)
    expect(canOpen('/system/fortigate', roles.network_engineer)).toBe(false)
    expect(canOpen('/requests', roles.viewer)).toBe(false)
    expect(canOpen('/segments/abc', roles.viewer)).toBe(true)
  })

  it('always opens the dashboard', () => {
    expect(canOpen('/', [])).toBe(true)
  })
})
