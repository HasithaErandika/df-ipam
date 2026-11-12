/**
 * Every permission the API checks (api/internal/auth/permissions.go), with what
 * it lets a person do. Roles are built from these in Users & roles.
 */
export interface PermissionInfo { key: string; label: string; detail: string }

export const permissionGroups: { group: string; items: PermissionInfo[] }[] = [
  {
    group: 'Network and inventory',
    items: [
      { key: 'inventory:view', label: 'See the network', detail: 'Discovery, servers, the IP register, devices, alerts and reservations.' },
      { key: 'inventory:edit', label: 'Change the register and devices', detail: 'Register and release addresses, approve or reject devices, import, and manage anyone\'s reservations.' },
      { key: 'network:manage', label: 'Set up the network', detail: 'Segments, environments and departments, including importing segments from text.' },
      { key: 'discovery:scan', label: 'Start scans', detail: 'Scan now, for all segments or one.' },
    ],
  },
  {
    group: 'Requests and alerts',
    items: [
      { key: 'ip:request', label: 'Ask for IP addresses', detail: 'Raise IP requests and see their own.' },
      { key: 'ip:reserve', label: 'Reserve addresses', detail: 'Hold an IP or a range for a purpose.' },
      { key: 'requests:approve', label: 'Approve IP requests', detail: 'Decide requests, and hear about new ones.' },
      { key: 'alerts:manage', label: 'Handle alerts', detail: 'Acknowledge alerts, and hear about new High and Critical ones.' },
    ],
  },
  {
    group: 'Reports and oversight',
    items: [
      { key: 'reports:export', label: 'Export reports', detail: 'Download any report as CSV, Excel or PDF.' },
      { key: 'audit:view', label: 'Read the audit log', detail: 'Who changed what and when, with export.' },
    ],
  },
  {
    group: 'Administration',
    items: [
      { key: 'users:manage', label: 'Manage users and roles', detail: 'Give people roles, switch access off, and set up roles.' },
      { key: 'system:manage', label: 'Manage the system', detail: 'Scan and check intervals, email, session timeout and the FortiGate connection.' },
    ],
  },
]

export const allPermissions = permissionGroups.flatMap((g) => g.items)

export const permissionLabel = (key: string) => allPermissions.find((p) => p.key === key)?.label ?? key
