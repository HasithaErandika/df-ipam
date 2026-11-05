

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

interface Slice { label: string; count: number }

export interface Distribution { environments: Slice[]; sites: Slice[]; families: Slice[] }

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
