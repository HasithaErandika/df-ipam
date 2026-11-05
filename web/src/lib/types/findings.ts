

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

export interface RunOutcome { detected: number; new: number; resolved: number; open: number }

export interface LastRun { ranAt: string | null; ranBy: string | null; new: number; resolved: number; open: number }

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

interface RuleWeights { reliability: number; freshness: number; agreement: number; cap: number; freshnessDays: number }

export interface FindingRule { classification: string; sourcePriority: string[]; weights: RuleWeights; isEnabled: boolean }

export interface EvidenceItem { source: string; mac: string | null; hostname: string | null; payload: unknown; observedAt: string }
