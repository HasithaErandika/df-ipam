/** Shapes the API returns. They mirror the Go DTOs in api/internal/modules. */

export type SegmentKind = 'dhcp' | 'server' | 'cloud' | 'vpn'
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
  alertPrivateMacs: boolean
  /** Checks a short list of common TCP ports on each device during scans. */
  checkPorts: boolean
  registered: number
  seen: number
  online: number
  poolSize: number
  poolUsed: number
  unknownDevices: number
  openAlerts: number
  lastSeen: string | null
  detectedGateway: string | null
  usableSize: number
  updatedAt: string
}

export interface Department {
  id: string
  name: string
  description: string
  devices: number
  addresses: number
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
  responseMs: number | null
  lastSeen: string | null
  seenMac: string | null
  updatedAt: string
  createdAt: string
  checkFailures: number
  deviceName: string | null
  /** The scanner's latest evidence at this address. */
  hostname: string
  nicVendor: string
  maker: string
  model: string
  deviceKind: string
  osHint: string
  vendor: string
  source: string
  openPorts: number[]
  openAlerts: number
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
  /** The scanner's latest evidence about it. */
  source: string
  nicVendor: string
  maker: string
  model: string
  deviceKind: string
  osHint: string
  openPorts: number[]
  isPrivateMac: boolean
  /** How many addresses it has been seen on. */
  addresses: number
  /** Its own register entry, by MAC or device. */
  registeredIp: string | null
  openAlerts: number
}

export interface ImportResult {
  /** True when nothing was kept: a preview of what the import would do. */
  dryRun: boolean
  imported: number
  skipped: number
  issues: { row: number; issue: string }[]
  /** The rows that go in: line in the file, key (IP or MAC) and a name. */
  preview: { row: number; key: string; label: string }[]
}
