import { useMutation } from '@tanstack/react-query'
import { Radar } from 'lucide-react'
import { Button } from '../../components/ui'
import { post } from '../../lib/api'
import { useInvalidate } from '../../lib/queries'
import { useCan } from '../../lib/session'

/**
 * Starts a scan now, for Admins. The server refuses while one runs and spaces
 * manual scans a minute apart, so this can't be used to flood the network.
 */
export function ScanButton({ segmentId, running, label = 'Scan now' }: { segmentId?: string; running?: boolean; label?: string }) {
  const can = useCan('system:manage')
  const refresh = useInvalidate('/discovery')
  const scan = useMutation({
    mutationFn: () => post('/discovery/scan', segmentId ? { segmentId } : {}),
    meta: { success: 'Scan started. Results appear as each segment finishes.' },
    onSuccess: () => void refresh(),
  })
  if (!can) return null
  return (
    <Button busy={scan.isPending || running} disabled={running} onClick={() => scan.mutate()} title={running ? 'A scan is running' : undefined}>
      {!running && !scan.isPending && <Radar size={14} aria-hidden />}{running ? 'Scanning…' : label}
    </Button>
  )
}
