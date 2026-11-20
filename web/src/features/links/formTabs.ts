/** The link dialog's tabs, and which fields (form or API names) each holds. */
export type TabKey = 'basic' | 'technical' | 'vpn' | 'related' | 'commercial' | 'notes'

export const tabLabels: Record<TabKey, string> = {
  basic: 'Basic', technical: 'Technical', vpn: 'VPN', related: 'Related links', commercial: 'Commercial', notes: 'Notes and files',
}

const fields: Record<TabKey, string[]> = {
  basic: ['name', 'linkType', 'providerId', 'circuitId', 'sourceSite', 'destination', 'purpose', 'environment', 'linkRole', 'criticality', 'officerId', 'status'],
  technical: ['downMbps', 'upMbps', 'medium', 'wanSegmentId', 'wanIp', 'extraWanIps', 'gatewayIp', 'publicIps', 'localSegmentIds', 'deviceIds', 'deviceText', 'interface', 'vlan', 'vrf', 'routing'],
  vpn: ['vpnTunnel', 'vpnType', 'localPeer', 'remotePeer', 'remoteSubnets', 'remoteOrg', 'remoteContact', 'lastTestDate', 'vaultRef'],
  related: ['relations'],
  commercial: ['noSeparateCost', 'billingAccount', 'monthlyRental', 'currency', 'installCost', 'contractRef', 'poRef', 'activationDate', 'contractStart', 'contractEnd', 'renewalNoticeDays', 'costCentre'],
  notes: ['remarks', 'files'],
}

/** The tab a field belongs to; unknown fields land on Basic. */
export const tabOf = (field: string): TabKey => (Object.keys(fields) as TabKey[]).find((t) => fields[t].includes(field)) ?? 'basic'

/** Tabs with at least one problem, in tab order. */
export const tabsWithErrors = (errors: Record<string, string>, tabs: TabKey[]) => tabs.filter((t) => Object.keys(errors).some((f) => tabOf(f) === t))
