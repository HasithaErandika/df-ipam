import { useQuery } from '@tanstack/react-query'
import { listAll } from '../../lib/api'

interface Option { id: string; name: string; code?: string; vlanId?: number; cidr?: string }

export const useOptions = (path: string, enabled = true) =>
  useQuery({ queryKey: ['options', path], enabled, queryFn: () => listAll<Option>(path) })

export const usePeople = (enabled = true) =>
  useQuery({ queryKey: ['options', '/people'], enabled, queryFn: () => listAll<{ id: string; email: string }>('/people') })
