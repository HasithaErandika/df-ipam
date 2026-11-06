import { useInvalidate } from '../../lib/queries'

export const methodLabels: Record<string, string> = {
  icmp: 'ICMP ping', arp: 'ARP table', ndp: 'IPv6 neighbours (NDP)', dns: 'Reverse DNS (passive)', snmp: 'SNMP', nmap: 'Nmap host discovery (-sn)',
}

export interface Window { days: number[]; start: string; end: string }

export interface Policy {
  id: string; name: string; description: string; methods: string[]; targetAllowlist: string[]; maxConcurrency: number; maxPps: number
  timeoutMs: number; retries: number; windows: Window[]; isSensitive: boolean; approvedByEmail: string | null; approvedAt: string | null; updatedAt: string
}

export interface Job {
  id: string; policyName: string | null; agentName: string | null; status: string; priority: number; progress: number; error: string | null
  config: { methods: string[]; targets: { routingDomainId: string; cidr: string }[]; skipped: string[]; maxPps: number; maxConcurrency: number }
  initiatedByEmail: string | null; scheduled: boolean; subnetName: string | null; targets: number; targetsDone: number; observations: number
  createdAt: string; startedAt: string | null; finishedAt: string | null
}

export interface Agent { id: string; name: string; siteName: string | null; status: string; version: string; lastHeartbeatAt: string | null; registrationPending: boolean; createdAt: string }

export interface Status { emergencyStop: boolean; changedAt: string | null; queued: number; running: number; paused: number; agentsOnline: number; agentsTotal: number }

export const jobTone: Record<string, 'neutral' | 'accent' | 'ok' | 'warn' | 'danger'> = {
  queued: 'neutral', running: 'accent', paused: 'warn', completed: 'ok', failed: 'danger', cancelled: 'neutral', stopped: 'danger',
}

export const useRefresh = () => useInvalidate('discovery', 'catalog')
