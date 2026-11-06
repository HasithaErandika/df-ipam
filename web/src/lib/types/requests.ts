

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

interface RequestResult {
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

interface WorkflowStep { stepOrder: number; approverRoleId: string; approverRoleName: string }

export interface Workflow {
  id: string
  name: string
  matchRules: { requestTypes?: string[]; environments?: string[]; sensitivities?: string[] }
  isActive: boolean
  steps: WorkflowStep[]
  inUse: number
}
