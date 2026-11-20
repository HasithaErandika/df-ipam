/** Bits a second as people read them: 850 kbps, 12.4 Mbps, 1.2 Gbps. */
export function formatRate(bps: number | null | undefined): string {
  if (bps == null) return '—'
  if (bps >= 1e9) return `${(bps / 1e9).toFixed(2)} Gbps`
  if (bps >= 1e6) return `${(bps / 1e6).toFixed(1)} Mbps`
  if (bps >= 1e3) return `${Math.round(bps / 1e3)} kbps`
  return `${Math.round(bps)} bps`
}

/** A rate as a share of the link's bandwidth (in Mbps), 0 to 100. */
export const share = (bps: number | null, mbps: number | null | undefined) => (bps == null || !mbps ? null : Math.min(100, (bps / (mbps * 1e6)) * 100))

/** Field names in a firewall difference, as people read them. */
export const firewallFields: Record<string, string> = {
  interface: 'Interface', wanIp: 'WAN IP', gatewayIp: 'Gateway IP', vlan: 'VLAN', downMbps: 'Download (Mbps)', upMbps: 'Upload (Mbps)',
  vpnTunnel: 'Tunnel name', localPeer: 'Local peer', remotePeer: 'Remote peer', remoteSubnets: 'Remote subnets',
}

export function shown(v: unknown): string {
  if (v === null || v === undefined || v === '') return 'none'
  if (Array.isArray(v)) return v.length ? v.join(', ') : 'none'
  return String(v)
}
