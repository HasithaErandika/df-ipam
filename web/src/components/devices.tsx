import {
  Apple, Bot, Cctv, CircleHelp, Cloud, Cpu, HardDrive, Laptop, Layers, Monitor, Network, Phone, Printer, Radio, Router, Server,
  Shield, Smartphone, Split, Tablet, Terminal, Wifi, type LucideIcon,
} from 'lucide-react'
import { Badge } from './ui'

export const deviceTypes: Record<string, { label: string; icon: LucideIcon }> = {
  firewall: { label: 'Firewall', icon: Shield },
  router: { label: 'Router', icon: Router },
  switch: { label: 'Switch', icon: Network },
  access_point: { label: 'Wireless access point', icon: Wifi },
  wireless_controller: { label: 'Wireless controller', icon: Radio },
  load_balancer: { label: 'Load balancer', icon: Split },
  network_appliance: { label: 'Network appliance', icon: Cpu },
  physical_server: { label: 'Server', icon: Server },
  virtual_machine: { label: 'Virtual machine', icon: Layers },
  cloud_interface: { label: 'Cloud interface', icon: Cloud },
  storage: { label: 'Storage', icon: HardDrive },
  laptop: { label: 'Laptop', icon: Laptop },
  desktop: { label: 'Desktop PC', icon: Monitor },
  mobile_phone: { label: 'Mobile phone', icon: Smartphone },
  tablet: { label: 'Tablet', icon: Tablet },
  ip_phone: { label: 'IP phone', icon: Phone },
  printer: { label: 'Printer', icon: Printer },
  camera: { label: 'Camera', icon: Cctv },
  iot: { label: 'IoT device', icon: Cpu },
  other: { label: 'Other device', icon: CircleHelp },
}

/** Four panes, the usual sign for Windows, drawn here so no logo file is needed. */
function Panes({ size = 14, className }: { size?: number; className?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" aria-hidden className={className} fill="currentColor">
      <rect x="1" y="1" width="6.5" height="6.5" rx=".6" /><rect x="8.5" y="1" width="6.5" height="6.5" rx=".6" />
      <rect x="1" y="8.5" width="6.5" height="6.5" rx=".6" /><rect x="8.5" y="8.5" width="6.5" height="6.5" rx=".6" />
    </svg>
  )
}

type Glyph = LucideIcon | typeof Panes

export const osFamilies: Record<string, { label: string; icon: Glyph }> = {
  windows: { label: 'Windows', icon: Panes },
  windows_server: { label: 'Windows Server', icon: Panes },
  linux: { label: 'Linux', icon: Terminal },
  macos: { label: 'macOS', icon: Apple },
  ios: { label: 'iOS', icon: Apple },
  android: { label: 'Android', icon: Bot },
  network_os: { label: 'Network OS', icon: Router },
  hypervisor: { label: 'Hypervisor', icon: Layers },
  other: { label: 'Other OS', icon: CircleHelp },
}

export function DeviceIcon({ type, size = 15 }: { type: string | null | undefined; size?: number }) {
  const def = deviceTypes[type ?? ''] ?? null
  if (!def) return <CircleHelp size={size} className="shrink-0 text-ink-3" aria-label="Device type unknown" />
  const Icon = def.icon
  return <Icon size={size} className="shrink-0 text-accent" aria-label={def.label} />
}

/** The operating system family, shown only when a record or reliable source states it. */
export function OSBadge({ family, version }: { family: string | null | undefined; version?: string | null }) {
  const def = osFamilies[family ?? '']
  if (!def) return version ? <span className="text-[12.5px] text-ink-2">{version}</span> : <span className="text-ink-3">—</span>
  const Icon = def.icon
  return (
    <span className="inline-flex items-center gap-1 text-[12.5px] text-ink-2" title={version ?? def.label}>
      <Icon size={13} className="text-ink-2" /> {def.label}
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
