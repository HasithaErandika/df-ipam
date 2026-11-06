import type { Severity } from '../format'
import type { Ref } from './core'

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

interface FreeRange { first: string; last: string; size: string; cidrs: string[] }

export interface FreeRanges { ranges: FreeRange[]; totalFree: string; rangeCount: number; truncated: boolean }

export interface GridCell {
  address: string
  kind: 'record' | 'candidate' | 'network' | 'broadcast' | 'gateway' | 'reserved_range' | 'dhcp'
  id?: string
  allocationState?: string
  observationState?: string
  hostname?: string
}
