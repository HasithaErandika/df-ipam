import { useInvalidate } from '../../lib/queries'

/** Invalidates everything a workflow change can affect. */
export const useRefresh = () => useInvalidate('requests', 'request', 'approvals', 'allocations', 'reservations', 'summary', 'notifications', 'subnets')
