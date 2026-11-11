import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, Plus } from 'lucide-react'
import { useState } from 'react'
import { Link, useParams } from 'react-router'
import { addressStates, AddressMap, StackedBar, type Run } from '../../components/charts'
import { Button, Card, EmptyState, EnvironmentBadge, ErrorState, PageHeader, SkeletonRows } from '../../components/ui'
import { get } from '../../lib/api'
import { formatCount, segmentCaption } from '../../lib/format'
import { label, segmentKinds } from '../../lib/labels'
import { useSegments } from '../../lib/queries'
import { useCan } from '../../lib/session'
import { RegisterIPModal, type RegisterPrefill } from '../shared/RegisterIPModal'
import { ScanButton } from '../shared/ScanButton'
import { SegmentAddresses } from './SegmentAddresses'

interface AddressMapData { usable: number; mapped: boolean; first?: string; counts: Record<string, number>; runs: Run[] }

const order = ['assigned', 'reserved', 'blocked', 'held', 'gateway', 'seen', 'dhcp_used', 'dhcp_free', 'available']

/** One segment: how its address space is used, as a map and a list. */
export function SegmentPage() {
  const { id = '' } = useParams()
  const segments = useSegments()
  const canEdit = useCan('inventory:edit')
  const map = useQuery({ queryKey: [`/segments/${id}/map`], queryFn: () => get<AddressMapData>(`/segments/${id}/map`), refetchInterval: 60_000 })
  const [prefill, setPrefill] = useState<RegisterPrefill | null>(null)
  const segment = segments.data?.find((s) => s.id === id)

  if (segments.isPending) return <SkeletonRows rows={8} />
  if (segments.isError) return <Card><ErrorState error={segments.error} onRetry={() => void segments.refetch()} /></Card>
  if (!segment) return <Card><EmptyState title="This segment no longer exists" body="It may have been removed." action={<Link className="text-accent hover:underline" to="/discovery">Back to Discovery</Link>} /></Card>

  const pick = (ip: string, state: string) => {
    if (canEdit && (state === 'available' || state === 'seen')) setPrefill({ segmentId: id, ip })
  }
  const parts = order.map((k) => ({ key: k, label: addressStates[k].label, value: map.data?.counts[k] ?? 0, color: addressStates[k].color }))
  const free = map.data?.counts.available ?? 0

  return (
    <>
      <Link to="/discovery" className="mb-2 inline-flex items-center gap-1 text-[12.5px] text-ink-3 hover:text-accent"><ArrowLeft size={13} aria-hidden />Discovery</Link>
      <PageHeader title={segment.name}
        description={[label(segmentKinds, segment.kind), segmentCaption(segment), segment.gateway ? `gateway ${segment.gateway}` : null, segment.dhcpStart ? `DHCP ${segment.dhcpStart} – ${segment.dhcpEnd}` : null].filter(Boolean).join(' · ')}
        actions={<>
          {segment.environmentName && <EnvironmentBadge name={segment.environmentName} />}
          {segment.kind !== 'cloud' && <ScanButton segmentId={id} label="Scan this segment" />}
          {canEdit && <Button variant="primary" onClick={() => setPrefill({ segmentId: id })}><Plus size={14} aria-hidden />Add to register</Button>}
        </>} />

      <Card title="Address space" subtitle={map.data ? `${formatCount(map.data.usable)} usable addresses · ${formatCount(free)} Available candidates` : undefined}>
        <div className="space-y-5 p-4">
          {map.isPending ? <SkeletonRows rows={3} /> : map.isError ? <ErrorState error={map.error} onRetry={() => void map.refetch()} /> : (
            <>
              <StackedBar label={`${segment.name} address use`} parts={parts.filter((p) => map.data.mapped ? true : p.value > 0)} height={14} />
              {map.data.mapped
                ? <AddressMap runs={map.data.runs} onPick={canEdit ? pick : undefined} />
                : <p className="text-[13px] text-ink-3">The address map is drawn for segments of 4,096 addresses or fewer. Use the list below.</p>}
            </>
          )}
        </div>
      </Card>

      <SegmentAddresses segmentId={id} onRegister={canEdit ? setPrefill : undefined} />
      <RegisterIPModal open={prefill !== null} prefill={prefill ?? undefined} onClose={() => setPrefill(null)} />
    </>
  )
}
