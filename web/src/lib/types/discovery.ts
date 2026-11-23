// What discovery found and how scanning stands: findings, address states, scan status.
import type { SegmentKind, RegisterStatus, DeviceStatus } from './inventory'

export interface Finding {
  id: string
  ip: string
  mac: string | null
  hostname: string
  vendor: string
  online: boolean
  source: string
  isStatic: boolean
  isPrivateMac: boolean
  isGateway: boolean
  firstSeen: string
  lastSeen: string
  segmentId: string | null
  segmentName: string | null
  segmentKind: SegmentKind | null
  registerId: string | null
  registerStatus: RegisterStatus | null
  assignedName: string | null
  deviceId: string | null
  deviceStatus: DeviceStatus | null
  deviceName: string | null
  owner: string | null
  /** Maker of the network card, from the MAC: often not the device's maker. */
  nicVendor: string
  maker: string
  model: string
  deviceKind: string
  /** Likely system family from the ping reply's TTL. A hint only. */
  osHint: string
  /** Common TCP ports found open in the last scan, when the segment checks them. */
  openPorts: number[]
  portsChecked: boolean
  /** Registered here, or an approved device registered at another address. */
  known: boolean
  /** Where this device is registered, when it is seen at an address that isn't. */
  homeIp: string | null
  homeSegment: string | null
  platform: string
  openAlerts: number
  /** The device's primary user from Microsoft Intune, matched by MAC or computer name. Empty when Intune doesn't know the device. */
  intuneUser: string
  intuneUserName: string
  intuneDevice: string
  intuneSeenAt: string | null
  intuneSerial: string
  intuneMaker: string
  intuneModel: string
  intuneOs: string
  intuneCompliance: string
  intuneEncrypted: boolean | null
  intuneLastSync: string | null
  /** From Microsoft Entra ID, matched by computer name: the device's registered owner, Entra name, join type ('AzureAd', 'ServerAd', 'Workplace') and last sign-in. */
  entraUser: string
  entraUserName: string
  entraDevice: string
  entraJoin: string
  entraLastSignIn: string | null
  entraSeenAt: string | null
  /** NetBIOS workgroup or Windows domain, and from the SMB handshake the DNS domain, full name and version ("10.0.22631"). */
  workgroup: string
  windowsDomain: string
  windowsFqdn: string
  osVersion: string
  /** A VPN client: the user the FortiGate signed in, 'ssl' or 'ipsec', the IPsec tunnel, and the public address it came from. */
  vpnUser: string
  vpnKind: string
  vpnTunnel: string
  vpnRemote: string
}

export interface SegmentAddress {
  ip: string
  registerId: string | null
  registerStatus: RegisterStatus | null
  assignedName: string | null
  owner: string | null
  mac: string | null
  hostname: string | null
  isStatic: boolean | null
  isPrivateMac: boolean | null
  isGateway: boolean
  source: string | null
  firstSeen: string | null
  lastSeen: string | null
  deviceId: string | null
  deviceStatus: DeviceStatus | null
  deviceName: string | null
  platform: string
  vendor: string
  inPool: boolean
  maker: string
  model: string
  deviceKind: string
  osHint: string
  openPorts: number[]
}

export interface AddressStatus {
  ip: string
  inSegment: boolean
  usable: boolean
  inDhcpPool: boolean
  isGateway: boolean
  registered: boolean
  registerName: string
  reserved: boolean
  seenRecently: boolean
  seenMac: string | null
  hostname: string
}

export interface ScanStatus {
  running: boolean
  /** The last scan was stopped by hand before it finished. */
  stopped: boolean
  trigger: string
  startedAt: string | null
  finishedAt: string | null
  nextScanAt: string | null
  method: string
  intervalMinutes: number
  autoScanDevices: boolean
  autoScanServers: boolean
  serverIntervalMinutes: number
  /** Probes a second, shared by sweeps and server checks. */
  ratePerSecond: number
  /** Addresses one round sweeps on user (DHCP) and server networks. */
  userAddresses: number
  serverAddresses: number
  segments: { id: string; name: string; found: number; error?: string; durationMs: number; scannedAt: string }[]
}
