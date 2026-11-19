import { useMutation } from '@tanstack/react-query'
import { Square } from 'lucide-react'
import { Button, useConfirm } from '../../components/ui'
import { post } from '../../lib/api'
import { useInvalidate } from '../../lib/queries'
import { useCan } from '../../lib/session'

/**
 * Stops the running scan, for those who may start one. Segments already swept
 * keep their results; the one in progress saves nothing.
 */
export function StopScanButton() {
  const can = useCan('discovery:scan')
  const confirm = useConfirm()
  const refresh = useInvalidate('/discovery', '/segments')
  const stop = useMutation({
    mutationFn: () => post('/discovery/stop', {}),
    meta: { success: 'Scan stopped. Segments already scanned keep their results.' },
    onSuccess: () => void refresh(),
  })
  if (!can) return null
  const ask = async () => {
    if (await confirm({
      title: 'Stop the scan?',
      body: 'Segments already scanned keep their results. The segment being scanned now saves nothing, and is scanned again next time. Recorded in the audit log.',
      confirmLabel: 'Stop scan',
      tone: 'danger',
    })) stop.mutate()
  }
  return (
    <Button busy={stop.isPending} onClick={() => void ask()}>
      {!stop.isPending && <Square size={13} aria-hidden />}Stop
    </Button>
  )
}
