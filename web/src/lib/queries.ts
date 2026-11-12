import { useQuery, useQueryClient } from '@tanstack/react-query'
import { get } from './api'
import type { Department, Environment, RoleDef, Segment } from './types'

/** Returns a function that marks out of date every query whose path starts with one of these. */
export function useInvalidate(...keys: string[]) {
  const queryClient = useQueryClient()
  return () => queryClient.invalidateQueries({
    predicate: (q) => typeof q.queryKey[0] === 'string' && keys.some((k) => (q.queryKey[0] as string).startsWith(k)),
  })
}

/** Every segment. Few and rarely changed, so cached for a minute and shared by all screens. */
export const useSegments = () => useQuery({ queryKey: ['/segments'], queryFn: () => get<Segment[]>('/segments'), staleTime: 60_000, refetchInterval: 60_000 })

export const useEnvironments = () => useQuery({ queryKey: ['/environments'], queryFn: () => get<Environment[]>('/environments'), staleTime: 60_000 })

/** Everything that shows register, device or alert state, refreshed after a change to any of them. */
export const inventoryKeys = ['/ip-addresses', '/devices', '/discovery', '/segments', '/dashboard', '/alerts', '/reservations', '/ip-requests']

/** Departments to pick from on devices and register entries. */
export const useDepartments = () => useQuery({ queryKey: ['/departments'], queryFn: () => get<Department[]>('/departments'), staleTime: 60_000 })

/** Roles and their permissions, for Users & roles. */
export const useRoles = () => useQuery({ queryKey: ['/roles'], queryFn: () => get<RoleDef[]>('/roles'), staleTime: 60_000 })
