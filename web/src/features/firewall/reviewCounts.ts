import type { FirewallInventory, FirewallReview } from '../../lib/types'

type Net = FirewallInventory['networks'][number]
type Dev = FirewallInventory['devices'][number]
type Host = FirewallInventory['servers'][number]
type Proposal = FirewallReview['proposals'][number]

/** What needs a decision on each tab: new, or different from DF-Atlas. */
export const netOpen = (n: Net) => n.canAdd || (Boolean(n.existing) && n.changes.length > 0)
export const devOpen = (d: Dev) => d.deviceStatus !== 'approved'
export const hostOpen = (h: Host) => !h.registered && Boolean(h.segmentId)
export const linkOpen = (p: Proposal) => !p.existing || p.changes.length > 0

/** How many items wait on each tab, or null before that part is fetched. */
export function openCounts(inv?: FirewallInventory, review?: FirewallReview) {
  return {
    networks: inv ? inv.networks.filter(netOpen).length : null,
    devices: inv ? inv.devices.filter(devOpen).length : null,
    servers: inv ? inv.servers.filter(hostOpen).length : null,
    links: review ? review.proposals.filter(linkOpen).length : null,
  }
}
