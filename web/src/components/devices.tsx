import { CircleHelp, Cctv, Cloud, Cpu, Database, Laptop, Layers, Monitor, Phone, Printer, Router, Server, Smartphone, Tablet, type LucideIcon } from 'lucide-react'
import { AccessPointGlyph, ApplianceGlyph, FirewallGlyph, LoadBalancerGlyph, RouterGlyph, SwitchGlyph, VirtualMachineGlyph, WirelessControllerGlyph } from './deviceIcons'
import { AndroidLogo, AppleLogo, LinuxLogo, WindowsLogo } from './osLogos'
import { Badge } from './ui'

type DeviceIconType = LucideIcon | typeof RouterGlyph

// The tile colour says what kind of thing it is: blue for network, amber for security.
const tiles = { network: 'bg-accent-soft text-accent', security: 'bg-warn-soft text-warn', compute: 'bg-surface-2 text-ink', endpoint: 'bg-surface-2 text-ink-2' }

export const deviceTypes: Record<string, { label: string; icon: DeviceIconType; kind: keyof typeof tiles }> = {
  firewall: { label: 'Firewall', icon: FirewallGlyph, kind: 'security' },
  router: { label: 'Router', icon: RouterGlyph, kind: 'network' },
  switch: { label: 'Switch', icon: SwitchGlyph, kind: 'network' },
  access_point: { label: 'Wireless access point', icon: AccessPointGlyph, kind: 'network' },
  wireless_controller: { label: 'Wireless controller', icon: WirelessControllerGlyph, kind: 'network' },
  load_balancer: { label: 'Load balancer', icon: LoadBalancerGlyph, kind: 'network' },
  network_appliance: { label: 'Network appliance', icon: ApplianceGlyph, kind: 'network' },
  physical_server: { label: 'Server', icon: Server, kind: 'compute' },
  virtual_machine: { label: 'Virtual machine', icon: VirtualMachineGlyph, kind: 'compute' },
  cloud_interface: { label: 'Cloud interface', icon: Cloud, kind: 'compute' },
  storage: { label: 'Storage', icon: Database, kind: 'compute' },
  laptop: { label: 'Laptop', icon: Laptop, kind: 'endpoint' },
  desktop: { label: 'Desktop PC', icon: Monitor, kind: 'endpoint' },
  mobile_phone: { label: 'Mobile phone', icon: Smartphone, kind: 'endpoint' },
  tablet: { label: 'Tablet', icon: Tablet, kind: 'endpoint' },
  ip_phone: { label: 'IP phone', icon: Phone, kind: 'endpoint' },
  printer: { label: 'Printer', icon: Printer, kind: 'endpoint' },
  camera: { label: 'Camera', icon: Cctv, kind: 'endpoint' },
  iot: { label: 'IoT device', icon: Cpu, kind: 'endpoint' },
  other: { label: 'Other device', icon: CircleHelp, kind: 'endpoint' },
}

type Glyph = LucideIcon | typeof WindowsLogo

export const osFamilies: Record<string, { label: string; icon: Glyph }> = {
  windows: { label: 'Windows', icon: WindowsLogo },
  windows_server: { label: 'Windows Server', icon: WindowsLogo },
  linux: { label: 'Linux', icon: LinuxLogo },
  macos: { label: 'macOS', icon: AppleLogo },
  ios: { label: 'iOS', icon: AppleLogo },
  android: { label: 'Android', icon: AndroidLogo },
  network_os: { label: 'Network OS', icon: Router },
  hypervisor: { label: 'Hypervisor', icon: Layers },
  other: { label: 'Other OS', icon: CircleHelp },
}

export function DeviceIcon({ type, size = 16 }: { type: string | null | undefined; size?: number }) {
  const def = deviceTypes[type ?? ''] ?? null
  const Icon = def?.icon ?? CircleHelp
  const box = size + 10
  return (
    <span role="img" aria-label={def?.label ?? 'Device type unknown'} title={def?.label} style={{ width: box, height: box }}
      className={`grid shrink-0 place-items-center rounded-[6px] ${tiles[def?.kind ?? 'endpoint']}`}>
      <Icon size={size} aria-hidden />
    </span>
  )
}

/** The operating system family, shown only when a record or reliable source states it. */
export function OSBadge({ family, version }: { family: string | null | undefined; version?: string | null }) {
  const def = osFamilies[family ?? '']
  if (!def) return version ? <span className="text-[12.5px] text-ink-2">{version}</span> : <span className="text-ink-3">—</span>
  const Icon = def.icon
  return (
    <span className="inline-flex items-center gap-1 text-[12.5px] text-ink-2" title={version ?? def.label}>
      <Icon size={14} className="shrink-0" /> {def.label}
    </span>
  )
}

export const allocationStates: Record<string, { label: string; tone: 'accent' | 'warn' | 'ok' | 'neutral' | 'danger' }> = {
  allocated: { label: 'Allocated', tone: 'accent' },
  reserved: { label: 'Reserved', tone: 'warn' },
  available_candidate: { label: 'Available candidate', tone: 'ok' },
  quarantined: { label: 'Quarantined', tone: 'warn' },
  retired: { label: 'Retired', tone: 'neutral' },
}

export const observationStates: Record<string, { label: string; tone: 'accent' | 'warn' | 'ok' | 'neutral' | 'danger' }> = {
  unknown: { label: 'Unknown', tone: 'neutral' },
  observed_active: { label: 'Observed active', tone: 'ok' },
  observed_inactive: { label: 'Observed inactive', tone: 'neutral' },
  conflict_suspected: { label: 'Conflict suspected', tone: 'danger' },
}

/** Allocation and observation are always two separate badges. */
export function StateBadges({ allocation, observation }: { allocation: string; observation: string }) {
  const a = allocationStates[allocation]
  const o = observationStates[observation]
  return (
    <span className="inline-flex flex-wrap gap-1">
      <Badge tone={a?.tone ?? 'neutral'}>{a?.label ?? allocation}</Badge>
      <Badge tone={o?.tone ?? 'neutral'}>{o?.label ?? observation}</Badge>
    </span>
  )
}
