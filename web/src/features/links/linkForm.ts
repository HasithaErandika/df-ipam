import type { LinkDetail } from '../../lib/types'

/** The link form as typed: every field a string, bandwidth with its unit. */
export interface LinkForm {
  name: string; linkType: string; providerId: string; circuitId: string; sourceSite: string; destination: string; purpose: string
  environment: string; linkRole: string; criticality: string; officerId: string; status: string
  down: string; downUnit: string; up: string; upUnit: string; medium: string; wanIp: string; wanSegmentId: string; publicIps: string
  gatewayIp: string; deviceId: string; deviceText: string; interface: string; vlan: string; vrf: string; routing: string
  vpnTunnel: string; vpnType: string; localPeer: string; remotePeer: string; remoteSubnets: string; remoteOrg: string
  remoteContact: string; lastTestDate: string; vaultRef: string; localSegmentIds: string[]
  noSeparateCost: boolean; billingAccount: string; monthlyRental: string; currency: string; installCost: string; contractRef: string
  poRef: string; activationDate: string; contractStart: string; contractEnd: string; renewalNoticeDays: string; costCentre: string
  supportContact: string; supportEmail: string; supportHotline: string; accountManager: string; slaPct: string; resolutionHours: string
  remarks: string; relations: { relatedId: string; kind: string }[]
}

export const emptyLink = (site = ''): LinkForm => ({
  name: '', linkType: '', providerId: '', circuitId: '', sourceSite: site, destination: '', purpose: '', environment: '', linkRole: 'primary',
  criticality: 'medium', officerId: '', status: 'active', down: '', downUnit: 'Mbps', up: '', upUnit: 'Mbps', medium: '', wanIp: '',
  wanSegmentId: '', publicIps: '', gatewayIp: '', deviceId: '', deviceText: '', interface: '', vlan: '', vrf: '', routing: '',
  vpnTunnel: '', vpnType: '', localPeer: '', remotePeer: '', remoteSubnets: '', remoteOrg: '', remoteContact: '', lastTestDate: '',
  vaultRef: '', localSegmentIds: [], noSeparateCost: false, billingAccount: '', monthlyRental: '', currency: 'LKR', installCost: '',
  contractRef: '', poRef: '', activationDate: '', contractStart: '', contractEnd: '', renewalNoticeDays: '', costCentre: '',
  supportContact: '', supportEmail: '', supportHotline: '', accountManager: '', slaPct: '', resolutionHours: '', remarks: '', relations: [],
})

const s = (v: string | number | null | undefined) => (v == null ? '' : String(v))

/** Bandwidth in Mbps as a number and unit: 1000 becomes 1 Gbps. */
function bandwidth(mbps: number | null): [string, string] {
  if (mbps == null) return ['', 'Mbps']
  return mbps >= 1000 && mbps % 1000 === 0 ? [String(mbps / 1000), 'Gbps'] : [String(mbps), 'Mbps']
}

export function fromDetail(d: LinkDetail): LinkForm {
  const l = d.link
  const [down, downUnit] = bandwidth(l.downMbps)
  const [up, upUnit] = bandwidth(l.upMbps)
  return {
    name: l.name, linkType: l.linkType, providerId: s(l.providerId), circuitId: l.circuitId, sourceSite: l.sourceSite, destination: l.destination,
    purpose: l.purpose, environment: l.environment, linkRole: l.linkRole, criticality: l.criticality, officerId: s(l.officerId), status: l.status,
    down, downUnit, up, upUnit, medium: l.medium, wanIp: s(l.wanIp), wanSegmentId: s(l.wanSegmentId), publicIps: s(l.publicIps).replace(/\/32$/, ''),
    gatewayIp: s(l.gatewayIp), deviceId: s(l.deviceId), deviceText: l.deviceText, interface: l.interface, vlan: s(l.vlan), vrf: l.vrf,
    routing: l.routing, vpnTunnel: l.vpnTunnel, vpnType: l.vpnType, localPeer: l.localPeer, remotePeer: l.remotePeer,
    remoteSubnets: l.remoteSubnets.join('\n'), remoteOrg: l.remoteOrg, remoteContact: l.remoteContact, lastTestDate: s(l.lastTestDate),
    vaultRef: l.vaultRef, localSegmentIds: d.localSegments.map((x) => x.id), noSeparateCost: l.noSeparateCost, billingAccount: l.billingAccount,
    monthlyRental: s(l.monthlyRental), currency: l.currency || 'LKR', installCost: s(l.installCost), contractRef: l.contractRef, poRef: l.poRef,
    activationDate: s(l.activationDate), contractStart: s(l.contractStart), contractEnd: s(l.contractEnd), renewalNoticeDays: s(l.renewalNoticeDays),
    costCentre: l.costCentre, supportContact: l.supportContact, supportEmail: l.supportEmail, supportHotline: l.supportHotline,
    accountManager: l.accountManager, slaPct: s(l.slaPct), resolutionHours: s(l.resolutionHours), remarks: l.remarks,
    relations: d.related.filter((r) => r.direction === 'outgoing').map((r) => ({ relatedId: r.id, kind: r.kind })),
  }
}

const num = (v: string) => (v.trim() === '' ? null : Number(v.replace(/,/g, '')))
const mbps = (v: string, unit: string) => {
  const n = num(v)
  return n == null ? null : unit === 'Gbps' ? n * 1000 : n
}

/** What the API takes (api/internal/modules/links/input.go). */
export function toPayload(f: LinkForm) {
  const { down, downUnit, up, upUnit, vlan, monthlyRental, installCost, renewalNoticeDays, slaPct, resolutionHours, remoteSubnets, ...rest } = f
  return {
    ...rest,
    downMbps: mbps(down, downUnit), upMbps: mbps(up, upUnit), vlan: num(vlan), monthlyRental: f.noSeparateCost ? null : num(monthlyRental),
    installCost: num(installCost), renewalNoticeDays: num(renewalNoticeDays), slaPct: num(slaPct), resolutionHours: num(resolutionHours),
    remoteSubnets: remoteSubnets.split(/[\s,;]+/).map((x) => x.trim()).filter(Boolean),
  }
}

/** Problems the browser can see before sending: required fields and numbers. */
export function clientIssues(f: LinkForm): Record<string, string> {
  const out: Record<string, string> = {}
  const required: [keyof LinkForm, string][] = [['name', 'Link name'], ['linkType', 'Link type'], ['sourceSite', 'Source location'], ['purpose', 'Business purpose']]
  for (const [k, label] of required) if (!String(f[k]).trim()) out[k] = `${label} is required.`
  const numbers: [keyof LinkForm, string, string][] = [['down', 'downMbps', 'Download bandwidth'], ['up', 'upMbps', 'Upload bandwidth'], ['vlan', 'vlan', 'VLAN ID'],
    ['monthlyRental', 'monthlyRental', 'Monthly rental'], ['installCost', 'installCost', 'Installation cost'], ['renewalNoticeDays', 'renewalNoticeDays', 'Renewal notice'],
    ['slaPct', 'slaPct', 'SLA availability'], ['resolutionHours', 'resolutionHours', 'Resolution time']]
  for (const [k, field, label] of numbers) {
    const v = String(f[k]).trim().replace(/,/g, '')
    if (v && !Number.isFinite(Number(v))) out[field] = `${label} must be a number.`
  }
  return out
}

/** Labels for the list of changes shown before an edit is saved. */
export const formLabels: Partial<Record<keyof LinkForm, string>> = {
  name: 'Link name', linkType: 'Link type', providerId: 'Service provider', circuitId: 'Circuit / service ID', sourceSite: 'Source location',
  destination: 'Destination', purpose: 'Business purpose', environment: 'Environment', linkRole: 'Link role', criticality: 'Criticality',
  officerId: 'Responsible officer', status: 'Status', down: 'Download bandwidth', downUnit: 'Download unit', up: 'Upload bandwidth', upUnit: 'Upload unit',
  medium: 'Connection medium', wanIp: 'WAN IP', wanSegmentId: 'WAN subnet', publicIps: 'Public IP / range', gatewayIp: 'Gateway IP',
  deviceId: 'Connected device', deviceText: 'Connected device (text)', interface: 'Interface / port', vlan: 'VLAN ID', vrf: 'VDOM / VRF',
  routing: 'Routing', vpnTunnel: 'VPN tunnel name', vpnType: 'VPN type', localPeer: 'Local peer', remotePeer: 'Remote peer',
  remoteSubnets: 'Remote subnets', remoteOrg: 'Remote organisation', remoteContact: 'Remote contact', lastTestDate: 'Last connectivity test',
  vaultRef: 'Credential vault reference', localSegmentIds: 'Local subnets', noSeparateCost: 'No separate cost', billingAccount: 'Billing account',
  monthlyRental: 'Monthly rental', currency: 'Currency', installCost: 'Installation cost', contractRef: 'Contract reference', poRef: 'PO reference',
  activationDate: 'Activation date', contractStart: 'Contract start', contractEnd: 'Contract expiry', renewalNoticeDays: 'Renewal notice',
  costCentre: 'Cost centre', supportContact: 'Support contact', supportEmail: 'Support email', supportHotline: 'Hotline',
  accountManager: 'Account manager', slaPct: 'SLA availability', resolutionHours: 'Resolution time', remarks: 'Remarks', relations: 'Related links',
}
