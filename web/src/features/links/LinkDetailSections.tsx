import { Link } from 'react-router'
import { Card, Details, StateBadge } from '../../components/ui'
import type { LinkDetail, LinkProvider, LinkSettings } from '../../lib/types'
import {
  bandwidthLine, criticalities, daysText, expiryTone, formatColombo, formatDay, formatMoney, linkEnvironments, linkRoles, linkStatuses,
  linkStatusTone, mediums, relationKinds, routings, typeName, vpnTypes,
} from './labels'

const or = (v: string | null | undefined) => (v ? v : null)
const mono = (v: string | null | undefined) => (v ? <span className="font-mono">{v}</span> : null)

function Block({ title, children }: { title: string; children: React.ReactNode }) {
  return <Card title={title}><div className="p-4">{children}</div></Card>
}

export function GeneralCard({ d, settings }: { d: LinkDetail; settings?: LinkSettings }) {
  const l = d.link
  return (
    <Block title="General information">
      <Details items={[
        ['Link ID', mono(l.code)], ['Link type', typeName(settings, l.linkType)],
        ['Service provider', l.providerName ? <Link className="text-accent hover:underline" to={`/network-links/providers?provider=${l.providerId}`}>{l.providerName}</Link> : null],
        ['Circuit / service ID', mono(l.circuitId)], ['Source location', l.sourceSite], ['Destination', or(l.destination)],
        ['Business purpose', l.purpose], ['Environment', l.environment ? linkEnvironments[l.environment] : null],
        ['Link role', linkRoles[l.linkRole]], ['Criticality', criticalities[l.criticality]],
        ['Responsible officer', l.officerEmail ? <span>{l.officerName ?? l.officerEmail}{l.officerName && <span className="text-ink-3"> · {l.officerEmail}</span>}</span> : null],
        ['Status', <StateBadge key="s" tone={linkStatusTone[l.status]} title="Administrative status, not live availability">{linkStatuses[l.status]}</StateBadge>],
        ['Added', formatColombo(l.createdAt)], ['Last changed', formatColombo(l.updatedAt)],
      ]} />
    </Block>
  )
}

export function TechnicalCard({ d }: { d: LinkDetail }) {
  const l = d.link
  return (
    <Block title="Technical configuration">
      <Details items={[
        ['Bandwidth', or(bandwidthLine(l.downMbps, l.upMbps))], ['Connection medium', l.medium ? mediums[l.medium] : null],
        ['WAN IP', mono(l.wanIp)],
        ['WAN subnet', l.wanSegmentId ? <Link className="text-accent hover:underline" to={`/segments/${l.wanSegmentId}`}>{l.wanSegment}</Link> : null],
        ['Public IP / range', mono(l.publicIps?.replace(/\/32$/, ''))], ['Gateway IP', mono(l.gatewayIp)],
        ['Connected device', l.deviceId ? <Link className="text-accent hover:underline" to={`/devices/all?q=${encodeURIComponent(l.deviceName ?? '')}&device=${l.deviceId}`}>{l.deviceName ?? 'Device'}</Link> : or(l.deviceText)],
        ['Interface / port', mono(l.interface)], ['VLAN ID', l.vlan ? String(l.vlan) : null], ['VDOM / VRF', or(l.vrf)],
        ['Routing', l.routing ? routings[l.routing] : null],
      ]} />
    </Block>
  )
}

export function VpnCard({ d }: { d: LinkDetail }) {
  const l = d.link
  return (
    <Block title="VPN information">
      <Details items={[
        ['Tunnel name', or(l.vpnTunnel)], ['VPN type', l.vpnType ? vpnTypes[l.vpnType] : null], ['Local peer', mono(l.localPeer)], ['Remote peer', mono(l.remotePeer)],
        ['Local subnets', d.localSegments.length ? <span className="flex flex-col">{d.localSegments.map((s) => <Link key={s.id} className="text-accent hover:underline" to={`/segments/${s.id}`}>{s.name} <span className="font-mono text-ink-3">{s.network}</span></Link>)}</span> : null],
        ['Remote subnets', l.remoteSubnets.length ? <span className="font-mono">{l.remoteSubnets.join(', ')}</span> : null],
        ['Remote organisation', or(l.remoteOrg)], ['Remote technical contact', or(l.remoteContact)],
        ['Last connectivity test', l.lastTestDate ? formatDay(l.lastTestDate) : null], ['Credential vault reference', or(l.vaultRef)],
      ]} />
    </Block>
  )
}

export function CommercialCard({ d }: { d: LinkDetail }) {
  const l = d.link
  if (l.commercialHidden) return <Block title="Commercial information"><p className="text-[13px] text-ink-3">Shown only to people who may see link costs and contracts.</p></Block>
  const days = l.contractEnd ? Math.round((Date.parse(l.contractEnd) - Date.parse(new Date().toLocaleDateString('en-CA', { timeZone: 'Asia/Colombo' }))) / 86_400_000) : null
  return (
    <Block title="Commercial information">
      <Details items={[
        ['Monthly rental', l.noSeparateCost ? 'No separate cost (paid through another link)' : l.monthlyRental == null ? null : formatMoney(l.monthlyRental, l.currency)],
        ['Annual recurring cost', l.noSeparateCost || l.annualCost == null ? null : formatMoney(l.annualCost, l.currency)],
        ['One-time installation cost', l.installCost == null ? null : formatMoney(l.installCost, l.currency)],
        ['Billing account', or(l.billingAccount)], ['Contract reference', or(l.contractRef)], ['Purchase order', or(l.poRef)],
        ['Activation date', l.activationDate ? formatDay(l.activationDate) : null], ['Contract start', l.contractStart ? formatDay(l.contractStart) : null],
        ['Contract expiry', l.contractEnd ? <span className="flex flex-wrap items-center gap-2">{formatDay(l.contractEnd)}{days != null && l.status !== 'terminated' && <StateBadge tone={expiryTone(days)}>{daysText(days)}</StateBadge>}</span> : null],
        ['Renewal notice', l.renewalNoticeDays != null ? `${l.renewalNoticeDays} days` : null], ['Cost centre', or(l.costCentre)],
      ]} />
    </Block>
  )
}

/** Support comes from the service provider. Details an older link holds for itself are shown too. */
export function SupportCard({ d, provider }: { d: LinkDetail; provider?: LinkProvider }) {
  const l = d.link
  const own = [l.supportContact, l.supportEmail, l.supportHotline, l.accountManager].some(Boolean) || l.slaPct != null || l.resolutionHours != null
  return (
    <Block title="Support">
      {!provider ? <p className="text-[13px] text-ink-3">No service provider. Choose one when editing to show who supports this link.</p> : (
        <Details items={[
          ['Provider', <Link key="p" className="text-accent hover:underline" to={`/network-links/providers?provider=${provider.id}`}>{provider.name}</Link>],
          ['Support hotline', or(provider.supportHotline)], ['Support email', or(provider.supportEmail)],
          ['Account manager', or(provider.accountManager)], ['Contact number', or(provider.contactNumber)], ['Email', or(provider.email)],
        ]} />
      )}
      {own && (
        <div className="mt-4 border-t border-line pt-3">
          <p className="mb-2 text-[12px] text-ink-3">Recorded for this link</p>
          <Details items={[
            ['Support contact', or(l.supportContact)], ['Support email', or(l.supportEmail)], ['Hotline', or(l.supportHotline)],
            ['Account manager', or(l.accountManager)], ['Agreed SLA availability', l.slaPct != null ? `${l.slaPct}%` : null],
            ['Agreed resolution time', l.resolutionHours != null ? `${l.resolutionHours} hours` : null],
          ]} />
        </div>
      )}
    </Block>
  )
}

export function AddressesCard({ d }: { d: LinkDetail }) {
  const uses: Record<string, string> = { wan: 'WAN IP', gateway: 'Gateway', public: 'Public range' }
  return (
    <Block title="Associated IP addresses and subnets">
      {d.registerEntries.length === 0 && !d.link.wanSegmentId && d.localSegments.length === 0
        ? <p className="text-[13px] text-ink-3">None of this link's addresses are in the IP register.</p>
        : (
          <ul className="space-y-1.5 text-[13px]">
            {d.link.wanSegmentId && <li>WAN subnet: <Link className="text-accent hover:underline" to={`/segments/${d.link.wanSegmentId}`}>{d.link.wanSegment}</Link></li>}
            {d.localSegments.map((s) => <li key={s.id}>Local subnet: <Link className="text-accent hover:underline" to={`/segments/${s.id}`}>{s.name}</Link> <span className="font-mono text-ink-3">{s.network}</span></li>)}
            {d.registerEntries.map((r) => (
              <li key={r.id}>{uses[r.use]}: <Link className="font-mono text-accent hover:underline" to={`/servers/register?q=${encodeURIComponent(r.ip)}&entry=${r.id}`}>{r.ip}</Link>
                {r.name && <span className="text-ink-2"> · {r.name}</span>} <span className="text-ink-3">in {r.segmentName}</span></li>
            ))}
          </ul>
        )}
    </Block>
  )
}

export function RelatedCard({ d, settings }: { d: LinkDetail; settings?: LinkSettings }) {
  const l = d.link
  return (
    <Block title="Primary and backup links">
      {d.related.length === 0 ? <p className="text-[13px] text-ink-3">No related links. Add backup or underlying links when editing.</p> : (
        <ul className="divide-y divide-line">
          {d.related.map((r) => (
            <li key={`${r.direction}-${r.id}`} className="flex flex-wrap items-center justify-between gap-2 py-2 text-[13px]">
              <span>
                <span className="text-ink-3">{r.direction === 'outgoing' ? `${relationKinds[r.kind]} of this link: ` : r.kind === 'underlying' ? 'Runs over this link: ' : `This link is its ${relationKinds[r.kind].toLowerCase()}: `}</span>
                <Link to={`/network-links/${r.id}`} className="font-medium text-accent hover:underline">{r.code} · {r.name}</Link>
                <span className="text-ink-3"> ({typeName(settings, r.linkType)}, {linkRoles[r.linkRole]?.toLowerCase()})</span>
              </span>
              <StateBadge tone={linkStatusTone[r.status]}>{linkStatuses[r.status]}</StateBadge>
            </li>
          ))}
        </ul>
      )}
      <p className="mt-2 text-[12px] text-ink-3">{l.code} is a {linkRoles[l.linkRole]?.toLowerCase()} link.</p>
    </Block>
  )
}
