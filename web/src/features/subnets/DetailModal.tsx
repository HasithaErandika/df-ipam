import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Button, ConfirmDialog, ErrorState, Modal, Mono, SkeletonRows } from '../../components/ui'
import { del, get, post } from '../../lib/api'
import { environments, formatCapacity, formatDateTime } from '../../lib/format'
import { useCan } from '../../lib/session'
import type { FreeRanges, Subnet, Utilisation } from '../../lib/types'
import { EditSubnetModal } from './EditSubnetModal'
import { Calculated, Detail } from './Calculated'

const heading = 'mb-1 text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase'

export function DetailModal({ subnet: initial, onClose }: { subnet: Subnet | null; onClose: () => void }) {
  const queryClient = useQueryClient()
  const canEdit = useCan('inventory:edit')
  const canDelete = useCan('inventory:delete')
  const [confirming, setConfirming] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [editing, setEditing] = useState(false)
  const [saved, setSaved] = useState<Subnet | null>(null)
  const subnet = saved?.id === initial?.id ? saved : initial
  const remove = useMutation({ meta: { success: 'Subnet deleted.', inlineError: true },
    mutationFn: (id: string) => del(`/subnets/${id}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['subnets'] })
      queryClient.invalidateQueries({ queryKey: ['summary'] })
      setDeleting(false)
      onClose()
    },
  })
  const archive = useMutation({ meta: { success: 'Subnet archived.', inlineError: true },
    mutationFn: (id: string) => post<void>(`/subnets/${id}/archive`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['subnets'] })
      queryClient.invalidateQueries({ queryKey: ['summary'] })
      setConfirming(false)
      onClose()
    },
  })
  const usage = useQuery({ queryKey: ['subnets', subnet?.id, 'utilisation'], enabled: subnet !== null, queryFn: () => get<Utilisation>(`/subnets/${subnet?.id}/utilisation`) })
  const free = useQuery({ queryKey: ['subnets', subnet?.id, 'free'], enabled: subnet !== null, queryFn: () => get<FreeRanges>(`/subnets/${subnet?.id}/free-ranges`) })
  if (!subnet) return null
  return (
    <>
      <Modal
        open
        size="xl"
        onClose={onClose}
        title={subnet.name}
        subtitle={<Mono>{subnet.calculated.cidr}</Mono>}
        footer={!subnet.archived && (canEdit || canDelete) ? <>
          {canDelete && <Button onClick={() => { remove.reset(); setDeleting(true) }}>Delete</Button>}
          {canEdit && <Button onClick={() => setConfirming(true)}>Archive subnet</Button>}
          {canEdit && <Button variant="primary" onClick={() => setEditing(true)}>Edit subnet</Button>}
        </> : undefined}
      >
        <div className="grid gap-5 md:grid-cols-2">
          <section>
            <h3 className={heading}>Capacity</h3>
            {usage.isPending && <SkeletonRows rows={5} />}
            {usage.error && <ErrorState error={usage.error} />}
            {usage.data && (
              <dl>
                <Detail label="Utilisation"><span className="tabular-nums">{usage.data.utilisationPct}%</span></Detail>
                <Detail label="Allocated"><span className="tabular-nums">{usage.data.allocated}</span></Detail>
                <Detail label="Reserved"><span className="tabular-nums">{usage.data.reserved}</span></Detail>
                <Detail label="Quarantined"><span className="tabular-nums">{usage.data.quarantined}</span></Detail>
                <Detail label="Reserved ranges"><span className="tabular-nums">{usage.data.reservedRangeAddresses}</span></Detail>
                <Detail label="DHCP pool"><span className="tabular-nums">{usage.data.dhcpPoolAddresses}</span></Detail>
                <Detail label="Available candidates"><span className="tabular-nums">{formatCapacity(usage.data.availableCandidates, subnet.calculated.family, subnet.calculated.prefixLength)}</span></Detail>
              </dl>
            )}
            <h3 className={`${heading} mt-4`}>Addressing</h3>
            <Calculated info={subnet.calculated} />
          </section>
          <section>
            <h3 className={heading}>Mapping</h3>
            <dl>
              <Detail label="Routing domain">{subnet.routingDomainName}</Detail>
              <Detail label="Environment">{environments[subnet.environment]}</Detail>
              <Detail label="Network block">{subnet.networkBlock?.name ?? 'Not set'}</Detail>
              <Detail label="Site">{subnet.site?.name ?? 'Not set'}</Detail>
              <Detail label="VLAN">{subnet.vlan?.name ?? 'Not set'}</Detail>
              <Detail label="VDOM">{subnet.vdom?.name ?? 'Not set'}</Detail>
              <Detail label="Business service">{subnet.businessService?.name ?? 'Not set'}</Detail>
              <Detail label="Technical owner">{subnet.technicalOwner?.name ?? 'Not set'}</Detail>
              <Detail label="Business owner">{subnet.businessOwner?.name ?? 'Not set'}</Detail>
              <Detail label="Gateway">{subnet.gateway ? <Mono>{subnet.gateway}</Mono> : 'Not set'}</Detail>
              <Detail label="Security zone">{subnet.securityZone || 'Not set'}</Detail>
              <Detail label="Updated">{formatDateTime(subnet.updatedAt)}</Detail>
            </dl>
            <h3 className={`${heading} mt-4`}>Free ranges</h3>
            {free.data?.ranges.length === 0 && <p className="text-[13px] text-ink-3">No free ranges.</p>}
            {free.data && free.data.ranges.length > 0 && (
              <ul className="max-h-40 overflow-y-auto">
                {free.data.ranges.map((r) => (
                  <li key={r.first} className="flex justify-between gap-3 border-b border-line py-1 last:border-0">
                    <Mono>{r.first === r.last ? r.first : `${r.first} – ${r.last}`}</Mono>
                  </li>
                ))}
              </ul>
            )}
            {free.data?.truncated && <p className="mt-1 text-[12px] text-ink-3">Showing the first {free.data.ranges.length} of {free.data.rangeCount}.</p>}
          </section>
        </div>
        {subnet.description && <p className="mt-4 text-ink-2">{subnet.description}</p>}
      </Modal>
      <ConfirmDialog
        open={confirming}
        title="Archive this subnet?"
        body={`${subnet.calculated.cidr} leaves the active inventory, and its range can be used again. History is kept.`}
        confirmLabel="Archive"
        busy={archive.isPending}
        error={archive.error}
        onConfirm={() => archive.mutate(subnet.id)}
        onCancel={() => setConfirming(false)}
      />
      <ConfirmDialog
        open={deleting}
        title="Delete this subnet?"
        body={`${subnet.calculated.cidr} is removed for good. Subnets with IP records or other links can't be deleted: archive them instead.`}
        confirmLabel="Delete"
        busy={remove.isPending}
        error={remove.error}
        onConfirm={() => remove.mutate(subnet.id)}
        onCancel={() => setDeleting(false)}
      />
      {editing && <EditSubnetModal subnet={subnet} onClose={() => setEditing(false)} onSaved={setSaved} />}
    </>
  )
}
