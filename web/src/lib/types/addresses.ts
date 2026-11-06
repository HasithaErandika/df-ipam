

export interface IPRecord {
  id: string
  address: string
  family: 4 | 6
  subnetId: string
  subnetName: string
  subnetCidr: string
  routingDomainId: string
  routingDomainName: string
  environment: string
  siteName: string | null
  hostname: string
  fqdn: string
  mac: string | null
  deviceId: string | null
  deviceName: string | null
  deviceType: string | null
  vendor: string | null
  operatingSystem: string | null
  osFamily: string | null
  applicationId: string | null
  applicationName: string | null
  businessServiceId: string | null
  businessServiceName: string | null
  ownerId: string | null
  ownerEmail: string | null
  allocationState: string
  observationState: string
  confidenceScore: number | null
  confidenceExplanation: string
  discoverySource: string | null
  firstSeen: string | null
  lastSeen: string | null
  lastVerified: string | null
  notes: string
  updatedAt: string
}

export interface DeviceInfo {
  address: string
  device: {
    id: string; name: string; hostname: string; deviceType: string; vendor: string; model: string; serialNumber: string | null
    osVersion: string; osFamily: string; managementIp: string | null; status: string; sensitivity: string; siteName: string | null
    sourceSystem: string; lastDiscovered: string | null; lastVerified: string | null; otherAddresses: number; businessService: string | null
  } | null
  interface: { name: string; mac: string | null; interfaceType: string; adminStatus: string; operStatus: string; vlan: string | null; vdom: string | null; source: string; lastObserved: string | null } | null
  mac: { address: string; locallyAdministered: boolean; multicast: boolean; oui: string } | null
  dnsRecords: { name: string; recordType: string; value: string; source: string | null; lastSeen: string | null }[]
  dhcpReservations: { scope: string; mac: string | null; hostname: string; source: string | null; lastSeen: string | null }[]
  sources: { source: string; reliability: number; observations: number; lastSeen: string }[]
  hasEvidence: boolean
}
