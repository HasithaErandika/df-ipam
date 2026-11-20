import { useQuery } from '@tanstack/react-query'
import { list } from '../../lib/api'
import type { Device } from '../../lib/types'

/** The register type for a device type. Phones are never registered by type, so they stay Other. */
const fromDeviceType: Record<string, string> = {
  pc: 'user_pc', laptop: 'user_pc', printer: 'printer', access_point: 'access_point', network: 'network', server: 'server', cctv: 'cctv',
}

export const ipTypeFromDevice = (deviceType: string | null | undefined) => (deviceType ? fromDeviceType[deviceType] : undefined)

/** The register type the evidence suggests, from the device icon's code (lib/platforms deviceIcon). */
const fromIcon: Record<string, string> = {
  camera: 'cctv', printer: 'printer', accesspoint: 'access_point', router: 'network', network: 'network', huawei: 'network',
  cisco: 'network', ubiquiti: 'network', mikrotik: 'network', fortinet: 'network', windows: 'user_pc', macos: 'user_pc',
}

export const ipTypeFromIcon = (code: string | null | undefined) => (code ? fromIcon[code] : undefined)

/**
 * The approved device with this MAC, if there is one. A register entry made for
 * its address starts from what people already recorded about it.
 */
export function useApprovedDevice(mac: string | null | undefined, enabled: boolean) {
  const wanted = mac?.toLowerCase() ?? ''
  return useQuery({
    queryKey: ['/devices', 'by-mac', wanted],
    queryFn: () => list<Device>(`/devices?status=approved&pageSize=5&q=${encodeURIComponent(wanted)}`),
    enabled: enabled && wanted !== '',
    select: (page) => page.data.find((d) => d.mac.toLowerCase() === wanted) ?? null,
    staleTime: 30_000,
  })
}

/** The register form fields an approved device can fill in. */
interface Fillable { deviceId: string; ipType: string; assignedName: string; platform: string; owner: string; department: string }

/**
 * What to change on a new register entry from the approved device: the link to
 * it, and every field the person has left as it started, including what
 * discovery filled in: what people recorded beats evidence. Their own edits stay.
 */
export function fillFromDevice(d: Device, edited: (field: keyof Fillable) => boolean): Partial<Fillable> {
  const out: Partial<Fillable> = { deviceId: d.id }
  const type = ipTypeFromDevice(d.deviceType)
  if (type && !edited('ipType')) out.ipType = type
  if (d.name && !edited('assignedName')) out.assignedName = d.name
  if (d.platform && !edited('platform')) out.platform = d.platform
  if (d.owner && !edited('owner')) out.owner = d.owner
  if (d.department && !edited('department')) out.department = d.department
  return out
}
