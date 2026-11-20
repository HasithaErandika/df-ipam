import { useQuery } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { Button, Field, MultiSearchSelect } from '../../components/ui'
import { get } from '../../lib/api'
import type { LinkOptions, SeenDevice } from '../../lib/types'
import { splitAddress } from '../../lib/validation'
import type { SectionProps } from './formParts'
import type { LinkForm } from './linkForm'

/** The addresses a link's devices answer on: its WAN IPs and gateway. */
export function linkIps(f: LinkForm): string[] {
  const all = [f.wanIp, f.gatewayIp, ...f.extraWan.map((r) => r.ip)].map((v) => splitAddress(v)?.ip).filter((v): v is string => Boolean(v))
  return [...new Set(all)].sort()
}

export const useSeenDevices = (ips: string[]) => useQuery({
  queryKey: ['/network-links', 'seen-devices', ...ips],
  queryFn: () => get<SeenDevice[]>(`/network-links/seen-devices?${ips.map((ip) => `ip=${encodeURIComponent(ip)}`).join('&')}`),
  enabled: ips.length > 0, staleTime: 60_000,
})

/**
 * The link's connected devices: approved devices, as many as it has. Devices
 * the scan saw on the link's addresses are found for you; on a new link the
 * approved ones are added until you change the list yourself. Saving is the
 * decision: nothing is linked without it.
 */
export function DevicesField({ p, options, known, isNew }: { p: SectionProps; options?: LinkOptions; known: { id: string; label: string }[]; isNew: boolean }) {
  const { f, set, errors } = p
  const seen = useSeenDevices(linkIps(f))
  const touched = useRef(!isNew)
  const [added, setAdded] = useState<string[]>([])
  const approved = (seen.data ?? []).filter((d) => d.status === 'approved')
  // Runs when the scan's answer changes. set is the form's setter: adding only
  // what isn't listed yet keeps this from repeating.
  useEffect(() => {
    if (touched.current) return
    const fresh = (seen.data ?? []).filter((d) => d.status === 'approved' && !f.deviceIds.includes(d.id))
    if (fresh.length === 0) return
    set('deviceIds', [...f.deviceIds, ...fresh.map((d) => d.id)])
    setAdded((a) => [...a, ...fresh.map((d) => `${d.name} (${d.ip})`)])
  }, [seen.data, f.deviceIds, set])
  const choices = [
    ...(options?.devices ?? []).map((o) => ({ value: o.id, label: o.label, detail: o.detail })),
    ...known.filter((k) => !options?.devices.some((o) => o.id === k.id)).map((k) => ({ value: k.id, label: k.label, detail: 'No longer approved' })),
  ]
  const change = (ids: string[]) => { touched.current = true; set('deviceIds', ids) }
  const offered = approved.filter((d) => !f.deviceIds.includes(d.id))
  const waiting = (seen.data ?? []).filter((d) => d.status !== 'approved')
  return (
    <div className="space-y-2 sm:col-span-2 lg:col-span-3">
      <Field label="Connected devices" htmlFor="link-deviceIds" error={errors.deviceIds}
        hint={added.length ? `Added from the scan: ${added.join(', ')}. Remove any that don't belong.` : 'Approved devices on this link: the firewall, the provider\'s router, a switch. Devices the scan saw on the WAN IPs and gateway are found for you.'}>
        <MultiSearchSelect id="link-deviceIds" values={f.deviceIds} onChange={change} options={choices} placeholder="Add a device…" />
      </Field>
      {seen.isError && <p className="text-[13px] text-danger">The scan's devices couldn't be read. Choose devices yourself.</p>}
      {offered.length > 0 && (
        <div className="flex flex-wrap items-center gap-2 text-[13px] text-ink-2">
          <span>Seen by the scan:</span>
          {offered.map((d) => (
            <Button key={d.id} onClick={() => change([...f.deviceIds, d.id])}><Plus size={14} aria-hidden />{d.name} <span className="font-mono text-ink-3">{d.ip}</span></Button>
          ))}
        </div>
      )}
      {waiting.length > 0 && (
        <p className="text-[13px] text-ink-3">Also seen, not approved yet: {waiting.map((d) => `${d.name} (${d.ip}, ${d.status})`).join(', ')}. Approve them in Devices to add them.</p>
      )}
    </div>
  )
}
