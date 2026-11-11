/** Shapes the API returns. They mirror the Go DTOs in api/internal/modules. */

export type SegmentKind = 'dhcp' | 'server' | 'cloud'
export type RegisterStatus = 'assigned' | 'reserved' | 'blocked'
export type Health = 'up' | 'down' | 'unknown'
export type DeviceStatus = 'pending' | 'approved' | 'rejected'
export type Severity = 'critical' | 'high' | 'medium' | 'low'

export interface Segment {
  id: string
  name: string
  kind: SegmentKind
  floor: number | null
  vlan: number | null
  network: string
  gateway: string | null
  dhcpStart: string | null
  dhcpEnd: string | null
  defaultEnvironmentId: string | null
  environmentName: string | null
  description: string
  registered: number
  seen: number
  usableSize: number
  updatedAt: string
}

export interface Environment {
  id: string
  name: string
  colour: string
  sortOrder: number
  defaultSeverity: Severity
  defaultMonitor: boolean
  defaultCritical: boolean
  inUse: number
}

export interface RegisterEntry {
  id: string
  segmentId: string
  segmentName: string
  segmentKind: SegmentKind
  ip: string
  status: RegisterStatus
  ipType: string
  environmentId: string | null
  environmentName: string | null
  environmentColour: string | null
  deviceId: string | null
  assignedName: string
  platform: string
  services: string[]
  expectedMac: string | null
  owner: string
  department: string
  monitor: boolean
  critical: boolean
  checkPort: number | null
  notes: string
  health: Health
  healthChangedAt: string | null
  lastSeen: string | null
  seenMac: string | null
  updatedAt: string
}

export interface Device {
  id: string
  mac: string
  name: string
  owner: string
  department: string
  deviceType: string
  platform: string
  status: DeviceStatus
  notes: string
  firstSeen: string | null
  lastSeen: string | null
  decidedAt: string | null
  lastIp: string | null
  segmentId: string | null
  segmentName: string | null
  hostname: string | null
  registered: boolean
  vendor: string
  online: boolean
}

export interface Finding {
  id: string
  ip: string
  mac: string | null
  hostname: string
  vendor: string
  online: boolean
  source: string
  isStatic: boolean
  isPrivateMac: boolean
  firstSeen: string
  lastSeen: string
  segmentId: string | null
  segmentName: string | null
  segmentKind: SegmentKind | null
  registerId: string | null
  registerStatus: RegisterStatus | null
  assignedName: string | null
  deviceId: string | null
  deviceStatus: DeviceStatus | null
  deviceName: string | null
  platform: string
  openAlerts: number
}

export interface SegmentAddress {
  ip: string
  registerId: string | null
  registerStatus: RegisterStatus | null
  assignedName: string | null
  owner: string | null
  mac: string | null
  hostname: string | null
  isStatic: boolean | null
  isPrivateMac: boolean | null
  source: string | null
  firstSeen: string | null
  lastSeen: string | null
  deviceId: string | null
  deviceStatus: DeviceStatus | null
  deviceName: string | null
  platform: string
  vendor: string
  inPool: boolean
}

export interface AddressStatus {
  ip: string
  inSegment: boolean
  usable: boolean
  inDhcpPool: boolean
  isGateway: boolean
  registered: boolean
  registerName: string
  reserved: boolean
  seenRecently: boolean
  seenMac: string | null
  hostname: string
}

export interface ScanStatus {
  running: boolean
  trigger: string
  startedAt: string | null
  finishedAt: string | null
  nextScanAt: string | null
  method: string
  intervalMinutes: number
  segments: { id: string; name: string; found: number; error?: string; durationMs: number; scannedAt: string }[]
}

export interface ImportResult {
  imported: number
  skipped: number
  issues: { row: number; issue: string }[]
}
