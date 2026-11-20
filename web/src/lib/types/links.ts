/** One network link (api/internal/modules/links/dto.go). Bandwidth is in Mbps. */
export interface NetworkLink {
  id: string; code: string; name: string; linkType: string
  providerId: string | null; providerName: string | null; circuitId: string
  sourceSite: string; destination: string; purpose: string; environment: string
  linkRole: string; criticality: string; officerId: string | null; officerEmail: string | null; officerName: string | null
  status: string; archivedAt: string | null
  downMbps: number | null; upMbps: number | null; medium: string; wanIp: string | null; wanSegmentId: string | null; wanSegment: string | null; extraWanIps: string[]
  publicIps: string | null; gatewayIp: string | null; deviceIds: string[]; deviceNames: string[]; deviceText: string
  interface: string; vlan: number | null; vrf: string; routing: string
  vpnTunnel: string; vpnType: string; localPeer: string; remotePeer: string; remoteSubnets: string[]
  remoteOrg: string; remoteContact: string; lastTestDate: string | null; vaultRef: string
  /** True when this person may not see commercial information: those fields are then empty. */
  commercialHidden: boolean
  noSeparateCost: boolean; billingAccount: string; monthlyRental: number | null; currency: string; installCost: number | null
  annualCost: number | null; contractRef: string; poRef: string; activationDate: string | null; contractStart: string | null
  contractEnd: string | null; renewalNoticeDays: number | null; costCentre: string
  supportContact: string; supportEmail: string; supportHotline: string; accountManager: string
  slaPct: number | null; resolutionHours: number | null; remarks: string
  createdAt: string; updatedAt: string
}

export interface LinkType { key: string; label: string; vpn: boolean; active: boolean }

export interface LinkSettings { types: LinkType[]; sites: string[]; reminderDays: number[]; usdRate: number | null }

export interface LinkProvider {
  id: string; name: string; accountManager: string; contactNumber: string; email: string; supportHotline: string
  supportEmail: string; website: string; remarks: string; active: boolean; links: number
}

export interface RelatedLink { id: string; code: string; name: string; linkType: string; linkRole: string; status: string; kind: string; direction: 'outgoing' | 'incoming' }

export interface LinkAttachment { id: string; category: string; fileName: string; contentType: string; size: number; uploadedBy: string | null; uploadedAt: string; removedAt: string | null }

export interface LinkDetail {
  link: NetworkLink
  related: RelatedLink[]
  localSegments: { id: string; name: string; network: string }[]
  registerEntries: { id: string; ip: string; name: string; segmentId: string; segmentName: string; use: string }[]
  attachments: LinkAttachment[]
  seenDevices: SeenDevice[]
}

/** A device the scan saw on one of a link's addresses in the last 30 days. Evidence, not inventory. */
export interface SeenDevice { id: string; name: string; mac: string; status: 'pending' | 'approved' | 'rejected'; ip: string; lastSeen: string }

export interface LinkOption { id: string; label: string; detail: string }

export interface LinkOptions { officers: LinkOption[]; segments: LinkOption[]; devices: LinkOption[]; links: LinkOption[]; registerIps: LinkOption[] }

export interface LinkHistoryEntry { at: string; actor: string; action: string; changes: { field: string; old: unknown; new: unknown }[] }

export interface LinkCount { key: string; label: string; count: number }
export interface LinkAmount { key: string; label: string; currency: string; amount: number }

export interface LinkRenewal {
  id: string; code: string; name: string; provider: string | null; contractEnd: string; daysLeft: number; noticeDays: number | null
  monthlyRental: number | null; currency: string; officer: string | null; contractRef: string
}

export interface LinkDashboard {
  totals: { total: number; active: number; inactive: number; pending: number; vpn: number; expiring: number }
  byType: LinkCount[]; byProvider: LinkCount[]; bySite: LinkCount[]; byStatus: LinkCount[]
  recent: { id: string; code: string; name: string; status: string; updatedAt: string }[]
  commercialHidden?: boolean
  monthlyByCurrency?: LinkAmount[]; monthlyByProvider?: LinkAmount[]; monthlyCombinedLkr?: number; renewals?: LinkRenewal[]
}

export interface LinkImportResult {
  dryRun: boolean; imported: number; added: number; updated: number; skipped: number
  issues: { row: number; issue: string }[]
  preview: { row: number; key: string; label: string; action: 'add' | 'update' }[]
}

/** A recorded link a firewall interface or tunnel belongs to. */
export interface FirewallLinkRef { id: string; code: string; name: string; downMbps: number | null; upMbps: number | null }

export interface FirewallLive {
  firewall: string; at: string
  interfaces: { name: string; alias: string; ip: string; role: string; up: boolean; speedMbps: number; rxBps: number | null; txBps: number | null; rxBytes: number; txBytes: number; errors: number; link: FirewallLinkRef | null }[]
  tunnels: { name: string; gateway: string; phaseUp: number; phases: number; rxBps: number | null; txBps: number | null; comments: string; link: FirewallLinkRef | null }[]
  health: { check: string; member: string; status: string; latencyMs: number; jitterMs: number; lossPct: number }[]
  vpnUsers: FirewallVPNUser[]
  cpu: number | null; memory: number | null; sessions: number | null
  problems: string[]
}

export interface FirewallSuggestion {
  name: string; linkType: string; interface: string; wanIp: string; gatewayIp: string; vlan: number | null; downMbps: number | null; upMbps: number | null
  deviceText: string; vrf: string; routing: string; vpnTunnel: string; vpnType: string; localPeer: string; remotePeer: string
  remoteSubnets: string[]; localSegments: { id: string; name: string; network: string }[]; underlying: string
}

export interface FirewallProposal {
  key: string; kind: 'interface' | 'ipsec' | 'dialup'; title: string
  facts: { label: string; value: string }[]; notes: string[]
  suggestion: FirewallSuggestion
  existing: FirewallLinkRef | null
  changes: { field: string; old: unknown; new: unknown }[]
}

export interface FirewallReview { firewall: string; at: string; proposals: FirewallProposal[] }

export interface FirewallApplyResult { key: string; action: 'add' | 'update'; ok: boolean; linkId?: string; code?: string; error?: string }

/** What the firewall knows, matched to DF-IPAM (GET /firewall/inventory). */
export interface FirewallInventory {
  firewall: string; at: string; problems: string[]
  networks: {
    key: string; interface: string; alias: string; role: string; name: string; kind: 'dhcp' | 'server' | 'vpn'; network: string; gateway: string
    vlan: number | null; dhcpStart: string; dhcpEnd: string; canAdd: boolean; notes: string[]
    existing: { id: string; name: string; kind: string; network: string } | null
    changes: { field: string; old: unknown; new: unknown }[]
  }[]
  devices: {
    mac: string; ip: string; hostname: string; os: string; vendor: string; kind: string; interface: string; online: boolean; sources: string[]
    privateMac: boolean; deviceType: string; platform: string; deviceId: string | null; deviceStatus: 'none' | 'pending' | 'approved' | 'rejected'; deviceName: string
  }[]
  servers: {
    ip: string; name: string; mac: string; sources: { kind: 'address' | 'vip' | 'reserved'; name: string; comment: string }[]
    segmentId: string; segmentName: string; segmentKind: string; ipType: string; notes: string[]
    registered: { id: string; ip: string; name: string } | null
  }[]
}

export interface FirewallVPNUser { user: string; kind: 'ssl' | 'ipsec'; tunnel: string; from: string; tunnelIp: string; since: string }
