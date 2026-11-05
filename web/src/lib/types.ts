import type { Severity } from './format'

export interface Me { id: string; email: string; roles: string[]; permissions: string[] }

export interface NetInfo {
  family: 4 | 6
  cidr: string
  network: string
  prefixLength: number
  mask?: string
  broadcast?: string
  firstUsable: string
  lastUsable: string
  totalAddresses: string
  usableAddresses: string
}

export interface Subnet {
  id: string
  routingDomainId: string
  routingDomainName: string
  name: string
  gateway: string | null
  environment: string
  securityZone: string
  description: string
  discoveryEnabled: boolean
  warnThreshold: number
  criticalThreshold: number
  sensitivity: string
  version: number
  archived: boolean
  createdAt: string
  updatedAt: string
  allocated: number
  reserved: number
  quarantined: number
  reservedRangeAddresses: string
  utilisationPct: string
  utilisationLevel: Severity
  calculated: NetInfo
  networkBlock: Ref | null
  site: Ref | null
  networkZone: Ref | null
  vdom: Ref | null
  vlan: Ref | null
  businessUnit: Ref | null
  businessService: Ref | null
  technicalOwner: Ref | null
  businessOwner: Ref | null
}

export interface Ref { id: string; name: string }

export interface Utilisation {
  totalAddresses: string
  usableAddresses: string
  allocated: number
  reserved: number
  quarantined: number
  reservedRangeAddresses: string
  dhcpPoolAddresses: string
  availableCandidates: string
  utilisationPct: string
  level: Severity
}

export interface FreeRange { first: string; last: string; size: string; cidrs: string[] }

export interface FreeRanges { ranges: FreeRange[]; totalFree: string; rangeCount: number; truncated: boolean }

export interface Finding {
  id: string
  classification: string
  objectType: string
  address: string | null
  routingDomainName: string | null
  sources: string[]
  evidenceFrom: string | null
  evidenceTo: string | null
  sourceReliability: number | null
  confidence: number | null
  explanation: string
  suggestedAction: string
  reviewStatus: string
  reviewNote: string
  reviewedBy: string | null
  lastDetectedAt: string
}

export interface FindingCount { classification: string; count: number }

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

export interface Summary {
  subnetsV4: number
  subnetsV6: number
  ipRecords: number
  allocated: number
  reserved: number
  availableCandidates: number
  observedActive: number
  unknownObservation: number
  conflictSuspected: number
  devices: number
  quarantined: number
  openFindings: number
}
