import { useQuery } from '@tanstack/react-query'
import { get, list } from '../../lib/api'
import { useCan } from '../../lib/session'

const minute = 60_000

/** Counts beside menu items: open alerts, devices waiting for a decision, and requests waiting for approval. */
export function useNavBadges(): Record<'alerts' | 'devices' | 'requests', number> {
  const canView = useCan('inventory:view')
  const canApprove = useCan('requests:approve')
  const alerts = useQuery({ queryKey: ['/alerts/summary'], queryFn: () => get<{ open: number }>('/alerts/summary'), enabled: canView, refetchInterval: minute })
  const devices = useQuery({ queryKey: ['/devices', 'pending-count'], queryFn: () => list('/devices?status=pending&pageSize=1'), enabled: canView, refetchInterval: minute })
  const requests = useQuery({ queryKey: ['/ip-requests', 'pending-count'], queryFn: () => list('/ip-requests?status=pending&pageSize=1'), enabled: canApprove, refetchInterval: minute })
  return { alerts: alerts.data?.open ?? 0, devices: devices.data?.meta.total ?? 0, requests: requests.data?.meta.total ?? 0 }
}
