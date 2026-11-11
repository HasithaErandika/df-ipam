export type Role = 'admin' | 'requester' | 'viewer'

/** The signed-in user. Role is null until an Admin gives one. */
export interface Me { id: string; email: string; role: Role | null; permissions: string[]; sessionTimeoutMinutes: number }

export interface Profile {
  id: string
  email: string
  displayName: string
  role: Role | null
  authProvider: string
  isActive: boolean
  lastLoginAt: string | null
  createdAt: string
  updatedAt: string
}

export const roleLabels: Record<Role, string> = { admin: 'Admin', requester: 'Requester', viewer: 'Viewer' }
