

interface ConnectorOption { key: string; label: string; required: boolean; hint?: string }

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
