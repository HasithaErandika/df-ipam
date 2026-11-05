import { useQuery } from '@tanstack/react-query'
import { type ReactNode } from 'react'
import { DeviceIcon, deviceTypes, OSBadge } from '../../components/devices'
import { Badge, ErrorState, Mono, SkeletonRows } from '../../components/ui'
import { Detail } from '../../components/workflow'
import { get } from '../../lib/api'
import { formatDateTime } from '../../lib/format'
import type { DeviceInfo } from '../../lib/types'

export const heading = 'mb-1 mt-4 text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase first:mt-0'

function Section({ title, children }: { title: string; children: ReactNode }) {
  return <section><h3 className={heading}>{title}</h3>{children}</section>
}

function Source({ children }: { children: ReactNode }) {
  return <span className="ml-1 rounded-[3px] bg-surface-2 px-1 text-[11px] text-ink-3">{children}</span>
}

/** Everything known about the device behind the address, each part with its source. */
export function DevicePanel({ id }: { id: string }) {
  const { data, isPending, error, refetch } = useQuery({ queryKey: ['ip', id, 'device'], queryFn: () => get<DeviceInfo>(`/ip-addresses/${id}/device`) })
  if (isPending) return <SkeletonRows rows={4} />
  if (error) return <ErrorState error={error} onRetry={refetch} />
  const nothing = !data.device && !data.interface && !data.mac && data.dnsRecords.length === 0 && data.dhcpReservations.length === 0 && !data.hasEvidence
  return (
    <div>
      {nothing && <p className="rounded-[5px] bg-surface-2 px-3 py-2 text-[13px] text-ink-2">Nothing is known about a device here yet. Link a device record, or wait for an approved source to report it.</p>}
      {data.device && (
        <div className="flex items-start gap-3 rounded-[5px] border border-line p-3">
          <div className="grid size-10 shrink-0 place-items-center rounded-md bg-accent-soft"><DeviceIcon type={data.device.deviceType} size={20} /></div>
          <div className="min-w-0 flex-1">
            <div className="font-medium text-ink">{data.device.name} <Source>{data.device.sourceSystem}</Source></div>
            <div className="text-[12.5px] text-ink-2">{deviceTypes[data.device.deviceType]?.label ?? data.device.deviceType}{[data.device.vendor, data.device.model].filter(Boolean).length > 0 && ` · ${[data.device.vendor, data.device.model].filter(Boolean).join(' ')}`}</div>
            <dl className="mt-2">
              <Detail label="Operating system"><OSBadge family={data.device.osFamily === 'unknown' ? null : data.device.osFamily} version={data.device.osVersion || null} />{data.device.osVersion && data.device.osFamily !== 'unknown' && <span className="ml-1 text-ink-3">{data.device.osVersion}</span>}</Detail>
              {data.device.hostname && <Detail label="Hostname">{data.device.hostname}</Detail>}
              {data.device.serialNumber && <Detail label="Serial number"><Mono>{data.device.serialNumber}</Mono></Detail>}
              {data.device.managementIp && <Detail label="Management IP"><Mono>{data.device.managementIp}</Mono></Detail>}
              {data.device.siteName && <Detail label="Site">{data.device.siteName}</Detail>}
              {data.device.businessService && <Detail label="Business service">{data.device.businessService}</Detail>}
              <Detail label="Status">{data.device.status}{data.device.sensitivity !== 'standard' && <> · <Badge tone="warn">{data.device.sensitivity}</Badge></>}</Detail>
              {data.device.otherAddresses > 0 && <Detail label="Other addresses">{data.device.otherAddresses} more on this device</Detail>}
              {data.device.lastVerified && <Detail label="Last verified">{formatDateTime(data.device.lastVerified)}</Detail>}
            </dl>
          </div>
        </div>
      )}
      {(data.interface || data.mac) && (
        <dl className="mt-3">
          {data.interface && <Detail label="Interface">{data.interface.name}{data.interface.vlan && ` · VLAN ${data.interface.vlan}`}{data.interface.vdom && ` · ${data.interface.vdom}`} <Source>{data.interface.source}</Source></Detail>}
          {data.interface && <Detail label="Link">{data.interface.adminStatus} / {data.interface.operStatus}</Detail>}
          {data.mac && (
            <Detail label="MAC">
              <Mono>{data.mac.address}</Mono>
              {data.mac.locallyAdministered && <span className="ml-1"><Badge>Locally administered</Badge></span>}
              <div className="text-[12px] text-ink-3">{data.mac.locallyAdministered ? 'Often a randomised phone or laptop address, or a virtual NIC.' : `Vendor prefix ${data.mac.oui}`}</div>
            </Detail>
          )}
        </dl>
      )}
      {data.dnsRecords.length > 0 && (
        <Section title="DNS names">
          <ul>{data.dnsRecords.map((r) => <li key={`${r.recordType}${r.name}`} className="flex justify-between border-b border-line py-1 text-[13px] last:border-0"><span><Badge>{r.recordType}</Badge> {r.name}</span><span className="text-ink-3">{r.source ?? 'DNS'}{r.lastSeen && ` · ${formatDateTime(r.lastSeen)}`}</span></li>)}</ul>
        </Section>
      )}
      {data.dhcpReservations.length > 0 && (
        <Section title="DHCP reservations">
          <ul>{data.dhcpReservations.map((r, i) => <li key={i} className="flex justify-between border-b border-line py-1 text-[13px] last:border-0"><span>{r.hostname || '—'} {r.mac && <Mono>{r.mac}</Mono>}</span><span className="text-ink-3">{r.scope}</span></li>)}</ul>
        </Section>
      )}
      {data.sources.length > 0 && (
        <Section title="Seen by">
          <ul>{data.sources.map((s) => <li key={s.source} className="flex justify-between border-b border-line py-1 text-[13px] last:border-0"><span>{s.source} <span className="text-ink-3">· {s.reliability}% reliable</span></span><span className="text-ink-3">{s.observations} reports · last {formatDateTime(s.lastSeen)}</span></li>)}</ul>
        </Section>
      )}
      <p className="mt-3 text-[12px] text-ink-3">From inventory records and approved read-only sources only. DF-IPAM doesn't probe devices from this view, and shows an operating system only when a record states it.</p>
    </div>
  )
}
