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
  objectId: string | null
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
  confidenceBreakdown: { reliability?: number; freshness?: number; agreement?: number; cap?: number }
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
  pendingApprovals: number
  readyToFulfil: number
  reservationsExpiringSoon: number
  highUtilisationSubnets: number
  integrationsHealthy: number
  integrationsFailing: number
  agentsOnline: number
  agentsOffline: number
  discoveryJobsRunning: number
  discoveryJobsFailedToday: number
}

/** Counts are strings, because IPv6 totals can pass what a JS number holds. Empty means no snapshot that day. */
export interface TrendPoint {
  date: string
  allocated: string
  reserved: string
  observedActive: string
  averagePct: string
  newFindings: number
  newConflicts: number
  observations: number
}

export interface Slice { label: string; count: number }
export interface Distribution { environments: Slice[]; sites: Slice[]; families: Slice[] }

export interface ConnectorOption { key: string; label: string; required: boolean; hint?: string }

export interface ConnectorKind {
  kind: string
  label: string
  available: boolean
  note: string
  needsUrl: boolean
  config: ConnectorOption[] | null
  credentials: ConnectorOption[] | null
}

export interface Connection {
  id: string
  kind: string
  name: string
  baseUrl: string
  config: Record<string, string>
  routingDomains: Record<string, string>
  hasCredentials: boolean
  isEnabled: boolean
  syncIntervalMinutes: number
  health: 'unknown' | 'healthy' | 'degraded' | 'failed'
  lastSuccessAt: string | null
  lastSyncAt: string | null
  lastError: string
  contexts: string[]
}

export interface SyncJob {
  id: string
  status: string
  counts: Record<string, number>
  error: string
  errorCount: number
  requestedBy: string | null
  createdAt: string
  startedAt: string | null
  finishedAt: string | null
}

export interface SyncError { code: string; message: string; occurredAt: string }

export interface ReportType {
  key: string
  group: string
  title: string
  description: string
  columns: string[]
  filters: string[]
  scoped: boolean
}

export interface ReportJob {
  id: string
  reportType: string
  title: string
  format: 'xlsx' | 'csv' | 'pdf'
  status: 'queued' | 'running' | 'completed' | 'failed'
  error: string
  rowCount: number
  sizeBytes: number
  truncated: boolean
  dataAsOf: string | null
  createdAt: string
  finishedAt: string | null
}

export interface ReportSummary {
  total: number
  ready: number
  inProgress: number
  failed: number
  lastReadyAt: string | null
  inventory: { subnets: number; ipRecords: number; allocated: number; reserved: number; devices: number; openFindings: number } | null
}

export interface MatrixRole {
  id: string
  name: string
  description: string
  isSystem: boolean
  locked: boolean
  isCustomised: boolean
  userCount: number
  permissions: string[]
}

export interface PermissionMatrix { permissions: string[]; roles: MatrixRole[] }

export interface SystemHealth {
  database: string
  workerHeartbeat: string | null
  workerHealthy: boolean
  encryptionConfigured: boolean
  emergencyStop: boolean
  lastBackup: string | null
  discoveryQueued: number
  syncsQueued: number
  reportsQueued: number
  emailPending: number
  agentsOnline: number
  agentsOffline: number
  integrationsHealthy: number
  integrationsFailing: number
}

export interface RetentionPolicy { dataClass: string; retainDays: number; updatedAt: string | null; updatedBy: string | null }

export interface ChangeRequest {
  id: string
  findingId: string | null
  finding: string | null
  objectType: string
  objectId: string | null
  proposed: Record<string, string>
  reason: string
  status: 'pending' | 'approved' | 'rejected' | 'applied'
  requestedBy: string | null
  requestedById: string | null
  decidedBy: string | null
  decisionNote: string
  createdAt: string
  decidedAt: string | null
}

export interface RuleWeights { reliability: number; freshness: number; agreement: number; cap: number; freshnessDays: number }

export interface FindingRule { classification: string; sourcePriority: string[]; weights: RuleWeights; isEnabled: boolean }

export type RequestType = 'allocation' | 'reservation' | 'release'

export interface Approval {
  id: string
  requestId: string
  stepOrder: number
  approverRoleId: string | null
  approverRoleName: string | null
  approverId: string | null
  approverEmail: string | null
  decision: 'pending' | 'approved' | 'rejected'
  comment: string
  decidedAt: string | null
}

export interface RequestResult {
  kind: 'allocation' | 'reservation'
  id: string
  address: string
  status: string
  expiresAt?: string | null
}

export interface IPRequest {
  id: string
  requestType: RequestType
  status: string
  subnetId: string
  subnetName: string
  subnetCidr: string
  routingDomainId: string
  routingDomainName: string
  selectionMode: 'automatic' | 'specific'
  requestedAddress: string | null
  allocationId: string | null
  reservationId: string | null
  applicationId: string | null
  applicationName: string | null
  businessServiceId: string | null
  businessServiceName: string | null
  technicalOwnerId: string | null
  technicalOwnerEmail: string | null
  businessOwnerId: string | null
  businessOwnerEmail: string | null
  justification: string
  department: string
  environment: string | null
  requiredDate: string | null
  expiryDate: string | null
  comments: string
  requestedBy: string
  requestedByEmail: string
  workflowId: string | null
  workflowName: string | null
  version: number
  createdAt: string
  updatedAt: string
  submittedAt: string | null
  decidedAt: string | null
  fulfilledAt: string | null
  fulfilledByEmail: string | null
  approvals: Approval[]
  result: RequestResult | null
  myApprovalId: string | null
}

export interface PendingApproval extends Approval { request: IPRequest }

export interface Allocation {
  id: string
  address: string
  ipAddressId: string
  subnetId: string
  subnetName: string
  subnetCidr: string
  routingDomainId: string
  routingDomainName: string
  status: 'active' | 'released'
  hostname: string
  ownerEmail: string | null
  requestId: string | null
  releaseRequestId: string | null
  openReleaseRequestId: string | null
  allocatedByEmail: string
  allocatedAt: string
  releasedByEmail: string | null
  releasedAt: string | null
}

export interface Reservation {
  id: string
  address: string
  ipAddressId: string
  subnetId: string
  subnetName: string
  subnetCidr: string
  routingDomainId: string
  routingDomainName: string
  status: 'active' | 'released' | 'expired' | 'converted'
  reason: string
  expiresAt: string | null
  requestId: string | null
  reservedBy: string
  reservedByEmail: string
  reservedAt: string
  releasedByEmail: string | null
  releasedAt: string | null
}

export interface WorkflowStep { stepOrder: number; approverRoleId: string; approverRoleName: string }

export interface Workflow {
  id: string
  name: string
  matchRules: { requestTypes?: string[]; environments?: string[]; sensitivities?: string[] }
  isActive: boolean
  steps: WorkflowStep[]
  inUse: number
}

export interface AppNotification {
  id: string
  eventType: string
  severity: 'info' | 'warning' | 'critical'
  title: string
  body: string
  link: string
  objectType: string | null
  objectId: string | null
  createdAt: string
  readAt: string | null
}

export interface NotificationPreference { eventType: string; label: string; channel: 'in_app' | 'email'; isEnabled: boolean }

export interface NotificationRule { eventType: string; label: string; recipients: string; severity: string; isEnabled: boolean }

export interface IPRecord {
  id: string
  address: string
  family: 4 | 6
  subnetId: string
  subnetName: string
  subnetCidr: string
  routingDomainId: string
  routingDomainName: string
  environment: string
  siteName: string | null
  hostname: string
  fqdn: string
  mac: string | null
  deviceId: string | null
  deviceName: string | null
  deviceType: string | null
  vendor: string | null
  operatingSystem: string | null
  osFamily: string | null
  applicationId: string | null
  applicationName: string | null
  businessServiceId: string | null
  businessServiceName: string | null
  ownerId: string | null
  ownerEmail: string | null
  allocationState: string
  observationState: string
  confidenceScore: number | null
  confidenceExplanation: string
  discoverySource: string | null
  firstSeen: string | null
  lastSeen: string | null
  lastVerified: string | null
  notes: string
  updatedAt: string
}

export interface GridCell {
  address: string
  kind: 'record' | 'candidate' | 'network' | 'broadcast' | 'gateway' | 'reserved_range' | 'dhcp'
  id?: string
  allocationState?: string
  observationState?: string
  hostname?: string
}

export interface DeviceInfo {
  address: string
  device: {
    id: string; name: string; hostname: string; deviceType: string; vendor: string; model: string; serialNumber: string | null
    osVersion: string; osFamily: string; managementIp: string | null; status: string; sensitivity: string; siteName: string | null
    sourceSystem: string; lastDiscovered: string | null; lastVerified: string | null; otherAddresses: number; businessService: string | null
  } | null
  interface: { name: string; mac: string | null; interfaceType: string; adminStatus: string; operStatus: string; vlan: string | null; vdom: string | null; source: string; lastObserved: string | null } | null
  mac: { address: string; locallyAdministered: boolean; multicast: boolean; oui: string } | null
  dnsRecords: { name: string; recordType: string; value: string; source: string | null; lastSeen: string | null }[]
  dhcpReservations: { scope: string; mac: string | null; hostname: string; source: string | null; lastSeen: string | null }[]
  sources: { source: string; reliability: number; observations: number; lastSeen: string }[]
  hasEvidence: boolean
}

export interface HistoryEntry { action: string; occurredAt: string; actorEmail: string | null; result: string }

export interface EvidenceItem { source: string; mac: string | null; hostname: string | null; payload: unknown; observedAt: string }

export interface SavedFilter { id: string; page: string; name: string; filters: Record<string, string> }
