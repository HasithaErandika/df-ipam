

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
