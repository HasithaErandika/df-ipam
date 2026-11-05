

export interface Me { id: string; email: string; roles: string[]; permissions: string[] }

export interface Ref { id: string; name: string }

export interface Organisation { id: string; name: string; code: string; createdAt: string }

export interface RoutingDomain {
  id: string
  organisationId: string
  organisationName: string
  name: string
  kind: string
  routeDistinguisher: string | null
  isIsolated: boolean
  subnetCount: number
}

export interface User {
  id: string
  email: string
  displayName: string
  isActive: boolean
  lastLoginAt: string | null
  roles: string[]
}

export interface Role { id: string; name: string; description: string }

export interface AuditEntry {
  id: number
  actorEmail: string | null
  actorType: string
  action: string
  objectType: string
  objectId: string | null
  oldValues: unknown
  newValues: unknown
  occurredAt: string
  sourceIp: string | null
  correlationId: string | null
  result: string
}

export interface HistoryEntry { action: string; occurredAt: string; actorEmail: string | null; result: string }

export interface SavedFilter { id: string; page: string; name: string; filters: Record<string, string> }
