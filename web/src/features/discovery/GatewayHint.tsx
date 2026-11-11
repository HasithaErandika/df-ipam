import { useMutation } from '@tanstack/react-query'
import { Router } from 'lucide-react'
import { Button, useConfirm } from '../../components/ui'
import { put } from '../../lib/api'
import { useInvalidate } from '../../lib/queries'
import { useCan } from '../../lib/session'
import type { Segment } from '../../lib/types'

/**
 * When discovery found this network's router but the segment doesn't record a
 * gateway (or records another), offer to set it. Discovery never changes the
 * segment by itself: a person confirms, and the change is audited.
 */
export function GatewayHint({ segment: s }: { segment: Segment }) {
  const canEdit = useCan('inventory:edit')
  const confirm = useConfirm()
  const refresh = useInvalidate('/segments', '/discovery')
  const save = useMutation({
    mutationFn: () => put(`/segments/${s.id}`, {
      name: s.name, kind: s.kind, floor: s.floor, vlan: s.vlan, network: s.network, gateway: s.detectedGateway,
      dhcpStart: s.dhcpStart ?? '', dhcpEnd: s.dhcpEnd ?? '', defaultEnvironmentId: s.defaultEnvironmentId ?? '', description: s.description, alertPrivateMacs: s.alertPrivateMacs,
    }),
    meta: { success: `Gateway set to ${s.detectedGateway}.` },
    onSuccess: () => void refresh(),
  })
  if (!s.detectedGateway || s.detectedGateway === s.gateway) return null
  const differs = Boolean(s.gateway)
  return (
    <div role="status" className="mb-4 flex flex-wrap items-center gap-3 rounded-md border border-line bg-surface px-4 py-3">
      <span className="grid size-8 place-items-center rounded-[5px] bg-[var(--addr-gateway)] text-white"><Router size={16} aria-hidden /></span>
      <div className="min-w-0 flex-1 text-[13px]">
        <p className="font-medium text-ink">
          {differs ? 'The network’s router answers at a different address' : 'Gateway detected'}: <span className="font-mono">{s.detectedGateway}</span>
        </p>
        <p className="text-ink-3">
          {differs ? `The segment records ${s.gateway}. ` : ''}It is the scanner's default route on this network, so it is most likely the router.
        </p>
      </div>
      {canEdit && (
        <Button busy={save.isPending} onClick={async () => {
          if (await confirm({ title: `Set ${s.detectedGateway} as the gateway?`, body: `${s.name} will record ${s.detectedGateway} as its gateway. The Available candidate skips it, and the address map shows it as the gateway. Recorded in the audit log.`, confirmLabel: 'Set as gateway', tone: 'primary' })) save.mutate()
        }}>Set as segment gateway</Button>
      )}
    </div>
  )
}
