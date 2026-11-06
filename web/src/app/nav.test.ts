import { describe, expect, it } from 'vitest'
import { canOpen, navFor } from './nav'

// The seeded roles, from api/internal/seed/seed.go.
const roles: Record<string, string[]> = {
  'Super Administrator': ['inventory:view', 'inventory:create', 'inventory:edit', 'inventory:delete', 'ip:request', 'ip:allocate', 'ip:reserve', 'requests:approve', 'imports:run', 'discovery:run', 'discovery:manage', 'integrations:manage', 'users:manage', 'audit:view', 'reports:export', 'system:manage'],
  'IPAM Administrator': ['inventory:view', 'inventory:create', 'inventory:edit', 'inventory:delete', 'ip:request', 'ip:allocate', 'ip:reserve', 'requests:approve', 'imports:run', 'discovery:run', 'discovery:manage', 'integrations:manage', 'audit:view', 'reports:export'],
  'Network Engineer': ['inventory:view', 'inventory:create', 'inventory:edit', 'ip:request', 'ip:allocate', 'ip:reserve', 'imports:run', 'discovery:run', 'reports:export'],
  'Infrastructure Engineer': ['inventory:view', 'inventory:edit_assigned', 'ip:request', 'reports:export'],
  'Security Auditor': ['inventory:view', 'audit:view', 'reports:export'],
  'Business Viewer': ['inventory:view_scoped', 'reports:export_scoped'],
}

const inventory = ['Network Management', 'Subnet Management', 'IP Address Explorer', 'Device Inventory', 'Interface Inventory']
const labels = (role: string) => navFor(roles[role]).flatMap((g) => g.items.map((i) => i.label))

describe('the menu follows the role', () => {
  it('gives Super Administrators everything', () => {
    expect(labels('Super Administrator')).toHaveLength(18)
  })

  it('keeps Network Engineers to their work', () => {
    expect(labels('Network Engineer')).toEqual(['Dashboard', ...inventory, 'Discovery Centre', 'Imports', 'Reconciliation Centre', 'IP Allocation Requests', 'Reservations', 'Alerts and Notifications', 'Reports'])
  })

  it('shows Infrastructure Engineers no discovery', () => {
    expect(labels('Infrastructure Engineer')).toEqual(['Dashboard', ...inventory, 'Reconciliation Centre', 'IP Allocation Requests', 'Reservations', 'Alerts and Notifications', 'Reports'])
  })

  it('lets Security Auditors read the evidence', () => {
    expect(labels('Security Auditor')).toEqual(['Dashboard', ...inventory, 'Discovery Centre', 'Imports', 'Reconciliation Centre', 'Integrations', 'IP Allocation Requests', 'Reservations', 'Alerts and Notifications', 'Reports', 'Audit Logs', 'Roles and Permissions', 'System Administration'])
  })

  it('offers Business Viewers only what works for them', () => {
    expect(labels('Business Viewer')).toEqual(['Dashboard', 'Alerts and Notifications', 'Reports'])
  })
})

describe('canOpen', () => {
  it('guards pages by address, tabs included', () => {
    expect(canOpen('/discovery/agents', roles['Infrastructure Engineer'])).toBe(false)
    expect(canOpen('/discovery/agents', roles['Security Auditor'])).toBe(true)
    expect(canOpen('/users', roles['IPAM Administrator'])).toBe(false)
    expect(canOpen('/subnets', roles['Business Viewer'])).toBe(false)
  })

  it('guards pages with no menu link', () => {
    expect(canOpen('/search', roles['Business Viewer'])).toBe(false)
    expect(canOpen('/search', roles['Network Engineer'])).toBe(true)
  })

  it('always opens the dashboard', () => {
    expect(canOpen('/', [])).toBe(true)
  })
})
