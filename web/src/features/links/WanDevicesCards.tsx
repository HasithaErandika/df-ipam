import { Link } from 'react-router'
import { Card, StateBadge } from '../../components/ui'
import { deviceStatuses, deviceTone, deviceTypes } from '../../lib/labels'
import type { LinkDetail, LinkWanAddress } from '../../lib/types'
import { formatColombo } from './labels'

const linkCls = 'text-accent hover:underline'

/** WAN addresses grouped by subnet, in the order they were entered: the main address's subnet first. */
export function wanGroups(list: LinkWanAddress[]) {
  const groups: { key: string; network: string | null; segmentId: string | null; segmentName: string | null; addresses: LinkWanAddress[] }[] = []
  for (const a of list) {
    const key = a.segmentId ?? a.network ?? 'none'
    const g = groups.find((x) => x.key === key)
    if (g) g.addresses.push(a)
    else groups.push({ key, network: a.network, segmentId: a.segmentId, segmentName: a.segmentName, addresses: [a] })
  }
  return groups
}

/** Every WAN subnet on the link, with the WAN addresses in each. */
export function WanAddressesCard({ d }: { d: LinkDetail }) {
  const groups = wanGroups(d.wanAddresses ?? [])
  const count = (d.wanAddresses ?? []).filter((a) => a.ip).length
  return (
    <Card title="WAN subnets and addresses" subtitle={count ? `${count} address${count === 1 ? '' : 'es'} in ${groups.length} subnet${groups.length === 1 ? '' : 's'}` : undefined}>
      {groups.length === 0
        ? <p className="p-4 text-[13px] text-ink-3">No WAN subnet or address recorded. Edit the link to add them.</p>
        : (
          <ul className="divide-y divide-line">
            {groups.map((g) => (
              <li key={g.key} className="space-y-1.5 px-4 py-3">
                <div className="flex flex-wrap items-baseline gap-x-2 text-[13px]">
                  {g.segmentId
                    ? <Link className={`font-medium ${linkCls}`} to={`/segments/${g.segmentId}`}>{g.segmentName}</Link>
                    : <span className="font-medium text-ink">{g.network ? 'Subnet' : 'No subnet'}</span>}
                  {g.network && <span className="font-mono text-ink-2">{g.network}</span>}
                  {!g.segmentId && g.network && <span className="text-[12px] text-ink-3">not a segment in the register</span>}
                </div>
                <ul className="space-y-1 pl-3">
                  {g.addresses.filter((a) => a.ip).map((a) => (
                    <li key={a.ip} className="flex flex-wrap items-baseline gap-x-2 text-[13px]">
                      {a.registerId
                        ? <Link className={`font-mono ${linkCls}`} to={`/servers/register?q=${encodeURIComponent(a.ip ?? '')}&entry=${a.registerId}`}>{a.ip}</Link>
                        : <span className="font-mono text-ink">{a.ip}</span>}
                      {a.main && <span className="rounded bg-accent-soft px-1.5 text-[11px] font-medium text-accent">Main</span>}
                      {a.registerName && <span className="text-ink-2">{a.registerName}</span>}
                      {!a.registerId && <span className="text-[12px] text-ink-3">not in the IP register</span>}
                    </li>
                  ))}
                  {g.addresses.every((a) => !a.ip) && <li className="text-[13px] text-ink-3">No WAN IP entered in this subnet.</li>}
                </ul>
              </li>
            ))}
          </ul>
        )}
    </Card>
  )
}

/** The link's connected devices, where the scan last saw each, and what else the scan saw on its addresses. */
export function DevicesCard({ d }: { d: LinkDetail }) {
  const devices = d.devices ?? []
  const listed = new Set(devices.map((x) => x.id))
  const others = (d.seenDevices ?? []).filter((x) => !listed.has(x.id))
  const empty = devices.length === 0 && !d.link.deviceText
  return (
    <Card title="Connected devices" subtitle={devices.length ? `${devices.length} device${devices.length === 1 ? '' : 's'}` : undefined}>
      {empty && <p className="p-4 text-[13px] text-ink-3">No devices recorded on this link.</p>}
      {devices.length > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full text-[13px]">
            <thead><tr className="bg-surface-2 text-left text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase">
              <th className="px-4 py-1.5">Device</th><th className="px-3 py-1.5">MAC</th><th className="px-3 py-1.5">Type</th>
              <th className="px-3 py-1.5">Status</th><th className="px-4 py-1.5">Last seen by the scan</th>
            </tr></thead>
            <tbody>
              {devices.map((x) => (
                <tr key={x.id} className="border-t border-line">
                  <td className="px-4 py-2"><Link className={linkCls} to={`/devices/all?q=${encodeURIComponent(x.mac)}&device=${x.id}`}>{x.name}</Link></td>
                  <td className="px-3 py-2 font-mono text-ink-2">{x.mac}</td>
                  <td className="px-3 py-2 text-ink-2">{deviceTypes[x.deviceType] ?? x.deviceType}</td>
                  <td className="px-3 py-2"><StateBadge tone={deviceTone[x.status] ?? 'neutral'}>{deviceStatuses[x.status] ?? x.status}</StateBadge></td>
                  <td className="px-4 py-2 text-ink-2">{x.lastIp ? <><span className="font-mono">{x.lastIp}</span>{x.lastSeen && <span className="text-ink-3"> · {formatColombo(x.lastSeen)}</span>}</> : 'Not seen'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {d.link.deviceText && <p className="border-t border-line px-4 py-3 text-[13px] text-ink-2"><span className="text-ink-3">Not in Devices: </span>{d.link.deviceText}</p>}
      {others.length > 0 && (
        <div className="border-t border-line px-4 py-3 text-[13px]">
          <p className="mb-1 text-ink-3">Also seen by the scan on this link's addresses (last 30 days), not recorded on the link:</p>
          <ul className="space-y-1">
            {others.map((x) => (
              <li key={x.id} className="flex flex-wrap items-baseline gap-x-2">
                <Link className={linkCls} to={`/devices/all?q=${encodeURIComponent(x.mac)}&device=${x.id}`}>{x.name}</Link>
                <span className="font-mono text-ink-3">{x.ip}</span>
                <StateBadge tone={deviceTone[x.status] ?? 'neutral'}>{deviceStatuses[x.status] ?? x.status}</StateBadge>
              </li>
            ))}
          </ul>
        </div>
      )}
    </Card>
  )
}
