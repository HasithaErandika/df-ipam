/** Words people see for values the API stores as codes. Unknown codes fall back to a readable form. */

export type Tone = 'neutral' | 'accent' | 'ok' | 'warn' | 'danger'

const readable = (code: string) => {
  const s = code.replaceAll('_', ' ')
  return s.charAt(0).toUpperCase() + s.slice(1)
}

export const label = (map: Record<string, string>, code: string | null | undefined) => (code ? map[code] ?? readable(code) : '')

export const ipTypes: Record<string, string> = {
  server: 'Server', printer: 'Printer', network: 'Network device', cctv: 'CCTV', access_point: 'Access point', user_pc: 'User PC', other: 'Other',
}

export const deviceTypes: Record<string, string> = {
  pc: 'PC', laptop: 'Laptop', printer: 'Printer', phone: 'Phone', access_point: 'Access point', network: 'Network device',
  server: 'Server', cctv: 'CCTV', other: 'Other',
}

export const registerStatuses: Record<string, string> = { assigned: 'Assigned', reserved: 'Reserved', blocked: 'Blocked' }

export const deviceStatuses: Record<string, string> = { pending: 'Pending', approved: 'Approved', rejected: 'Rejected' }

export const requestStatuses: Record<string, string> = { pending: 'Pending', approved: 'Approved', rejected: 'Rejected', cancelled: 'Cancelled' }

export const severities: Record<string, string> = { critical: 'Critical', high: 'High', medium: 'Medium', low: 'Low' }

export const alertTypes: Record<string, string> = {
  unknown_device: 'Unknown device', unregistered_static: 'Unregistered static IP', register_misuse: 'Registered IP misused',
  unknown_server_ip: 'Unknown server IP', ip_conflict: 'IP conflict', server_down: 'Server down',
  server_mac_changed: 'Server MAC changed', rejected_device: 'Rejected device back', private_mac_device: 'Private-MAC device',
}

export const alertStatuses: Record<string, string> = { open: 'Open', acknowledged: 'Acknowledged', closed: 'Closed' }

export const segmentKinds: Record<string, string> = { dhcp: 'DHCP network', server: 'Server', cloud: 'Cloud' }

export const sources: Record<string, string> = { arp: 'ARP', ping: 'Ping', netbios: 'NetBIOS', snmp: 'SNMP', dns: 'DNS', dhcp: 'DHCP lease', vpn: 'VPN' }

export const notificationKinds: Record<string, string> = {
  alert: 'High and Critical alerts', server_up: 'Server back up', request: 'New IP requests (Admins)',
  request_decision: 'Decisions on my IP requests', reservation_expiry: 'Reservations about to expire',
}

export const severityTone: Record<string, Tone> = { critical: 'danger', high: 'warn', medium: 'accent', low: 'neutral' }

export const healthTone: Record<string, Tone> = { up: 'ok', down: 'danger', unknown: 'neutral' }

export const registerTone: Record<string, Tone> = { assigned: 'accent', reserved: 'warn', blocked: 'danger' }

export const deviceTone: Record<string, Tone> = { pending: 'warn', approved: 'ok', rejected: 'danger' }

export const requestTone: Record<string, Tone> = { pending: 'warn', approved: 'ok', rejected: 'danger', cancelled: 'neutral' }

/** Turns a code map into select options. */
export const options = (map: Record<string, string>) => Object.entries(map).map(([value, text]) => ({ value, text }))
