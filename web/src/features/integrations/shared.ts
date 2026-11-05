

export const healthTone = { healthy: 'ok', degraded: 'warn', failed: 'danger', unknown: 'neutral' } as const

export const healthLabel = { healthy: 'Healthy', degraded: 'Degraded', failed: 'Failed', unknown: 'Not synced yet' }

export const jobTone = (s: string) => (s === 'completed' ? 'ok' as const : s === 'failed' ? 'danger' as const : 'accent' as const)

export interface Form {
  kind: string
  name: string
  baseUrl: string
  config: Record<string, string>
  credentials: Record<string, string>
  syncIntervalMinutes: number
  isEnabled: boolean
}

export const blank: Form = { kind: 'fortigate', name: '', baseUrl: '', config: {}, credentials: {}, syncIntervalMinutes: 60, isEnabled: true }
