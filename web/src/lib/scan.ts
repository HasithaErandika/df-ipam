import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { get } from './api'
import { useInvalidate } from './queries'
import type { ScanStatus } from './types'

/**
 * The scanner's state: checked every 3 seconds while a scan runs and every 20
 * otherwise. When a scan finishes, everything that shows its results reloads at
 * once, so new devices appear without waiting or refreshing the page.
 */
export function useScanStatus() {
  const refresh = useInvalidate('/discovery/findings', '/segments', '/dashboard', '/devices', '/alerts', '/ip-addresses')
  const q = useQuery({
    queryKey: ['/discovery/status'],
    queryFn: () => get<ScanStatus>('/discovery/status'),
    refetchInterval: (x) => (x.state.data?.running ? 3_000 : 20_000),
  })
  const finished = q.data?.finishedAt
  const seen = useRef(finished)
  useEffect(() => {
    if (finished && seen.current && finished !== seen.current) void refresh()
    seen.current = finished
    // refresh changes every render; only a new finish time should trigger this.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [finished])
  return q
}
