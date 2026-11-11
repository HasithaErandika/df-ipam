import type { Role } from './core'
import type { Severity } from './inventory'

export type RequestStatus = 'pending' | 'approved' | 'rejected' | 'cancelled'

export interface IPRequest {
  id: string
  requesterId: string
  requesterEmail: string
  segmentId: string
  segmentName: string
  environmentId: string | null
  environmentName: string | null
  ipType: string
  name: string
  purpose: string
  neededBy: string | null
  requestedIp: string | null
  status: RequestStatus
  decisionReason: string
  decidedByEmail: string | null
  decidedAt: string | null
  assignedIp: string | null
  createdAt: string
}

export interface Reservation {
  id: string
  segmentId: string
  segmentName: string
  startIp: string
  endIp: string
  size: number
  reason: string
  ownerId: string | null
  ownerName: string
  expiresAt: string | null
  status: 'active' | 'converted' | 'released'
  createdByEmail: string | null
  createdAt: string
}

export interface Alert {
  id: string
  type: string
  severity: Severity
  status: 'open' | 'acknowledged' | 'closed'
  message: string
  segmentId: string | null
  segmentName: string | null
  ipAddressId: string | null
  deviceId: string | null
  ip: string | null
  mac: string | null
  openedAt: string
  lastSeenAt: string
  acknowledgedBy: string | null
  acknowledgedAt: string | null
  closedAt: string | null
}

export interface Notice {
  id: string
  kind: string
  message: string
  link: string
  isRead: boolean
  emailStatus: string
  createdAt: string
}

export interface User {
  id: string
  email: string
  displayName: string
  role: Role | null
  isActive: boolean
  lastLoginAt: string | null
  createdAt: string
}

export interface AuditEntry {
  id: number
  occurredAt: string
  actorEmail: string | null
  actorType: string
  action: string
  objectType: string
  objectId: string | null
  oldValues: unknown
  newValues: unknown
  sourceIp: string | null
  correlationId: string | null
}

export interface SystemSettings {
  values: Record<string, number>
  emailEnabled: boolean
  smtpConfigured: boolean
  scanMethod: string
}

export interface ReportDefinition {
  key: string
  title: string
  description: string
  dated: boolean
  environment: boolean
  columns: string[]
}

export interface Dashboard {
  floors: { id: string; name: string; floor: number | null; vlan: number | null; network: string; registered: number; seen: number; poolUsed: number; static: number; poolSize: number }[]
  alerts: { severity: Severity; count: number }[]
  servers: { environment: string; colour: string; total: number; up: number; down: number; unknown: number }[]
  totals: { pendingDevices: number; pendingRequests: number; expiringReservations: number; registered: number; assigned: number; reserved: number; blocked: number; held: number; online: number; unknownDevices: number }
}
