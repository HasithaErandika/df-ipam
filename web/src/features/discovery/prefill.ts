import { deviceIcon, knownPlatform, likelyPlatform } from '../../lib/platforms'
import type { Finding } from '../../lib/types'
import type { DecisionTarget } from '../shared/DeviceDecision'
import type { RegisterPrefill } from '../shared/RegisterIPModal'
import { ipTypeFromIcon } from '../shared/registerFromDevice'
import { deviceFacts, iconEvidence } from './deviceInfo'
import { windowsName } from './windowsInfo'

/** Who uses the device, best source first: Intune, Entra ID, then the VPN sign-in. */
export function findingUser(r: Finding): string {
  return r.intuneUserName || r.intuneUser || r.entraUserName || r.entraUser || r.vpnUser || ''
}

/** The device type the evidence suggests, from the device icon's code. */
const typeFromIcon: Record<string, string> = {
  camera: 'cctv', printer: 'printer', accesspoint: 'access_point', router: 'network', network: 'network', huawei: 'network',
  cisco: 'network', ubiquiti: 'network', mikrotik: 'network', fortinet: 'network', windows: 'pc', macos: 'laptop',
  windows_server: 'server', ios: 'phone', android: 'phone',
}

/** What the scanner and Intune found, as one line for the register's notes. */
function evidenceNote(r: Finding): string {
  const f = deviceFacts(r)
  const model = [r.intuneMaker || f.maker, r.intuneModel || f.model].filter(Boolean).join(' ')
  const parts = [
    model, r.intuneSerial && `serial ${r.intuneSerial}`, r.intuneOs || (r.osVersion ? windowsName(r.osVersion) : ''),
    (r.windowsDomain || r.workgroup) && `domain ${r.windowsDomain || r.workgroup}`,
  ].filter(Boolean)
  return parts.length ? `From discovery: ${[...new Set(parts)].join(', ')}.` : ''
}

const name = (r: Finding) => r.deviceName || r.hostname || r.intuneDevice || r.entraDevice || null
const platform = (r: Finding) => knownPlatform(r.platform) || likelyPlatform(r) || ''
const icon = (r: Finding) => deviceIcon(iconEvidence(r))?.code

/** A new register entry for this address, filled in from everything discovery knows. */
export function registerPrefill(r: Finding): RegisterPrefill {
  return {
    segmentId: r.segmentId ?? undefined, ip: r.ip, mac: r.mac, name: name(r), platform: platform(r), platformGuess: likelyPlatform(r),
    ipType: ipTypeFromIcon(icon(r)), owner: findingUser(r), notes: evidenceNote(r),
  }
}

/** The approval dialog for this device, filled in from everything discovery knows. */
export function approveTarget(r: Finding): DecisionTarget {
  const code = icon(r)
  return {
    id: r.deviceId!, mac: r.mac!, name: name(r), hostname: r.hostname, vendor: r.vendor, model: r.model, osHint: r.osHint,
    platform: platform(r), owner: findingUser(r), deviceType: code ? typeFromIcon[code] : undefined, ipType: ipTypeFromIcon(code),
    lastIp: r.ip, segmentId: r.segmentId, segmentName: r.segmentName, registered: Boolean(r.registerId),
  }
}
