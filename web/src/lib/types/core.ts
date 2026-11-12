/** A role's key, such as admin or network_engineer. Roles are set up in Users & roles. */
export type Role = string

/** The signed-in user. Role is null until an Admin gives one. */
export interface Me { id: string; email: string; role: Role | null; roleName: string; permissions: string[]; sessionTimeoutMinutes: number }

export interface Profile {
  id: string
  email: string
  displayName: string
  role: Role | null
  roleName: string | null
  authProvider: string
  isActive: boolean
  lastLoginAt: string | null
  createdAt: string
  updatedAt: string
}

/** A role and the permissions it grants. A full-access role holds every permission. */
export interface RoleDef {
  key: string
  name: string
  description: string
  permissions: string[]
  fullAccess: boolean
  isSystem: boolean
  users: number
}
