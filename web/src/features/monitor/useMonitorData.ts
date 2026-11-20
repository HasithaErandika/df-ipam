import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { get } from '../../lib/api'
import { useCan } from '../../lib/session'
import type { Dashboard, LinkDashboard, Segment } from '../../lib/types'
import { useFirewallLive } from '../firewall/LiveView'
import { useLinkSettings } from '../links/queries'

const history = 90 // samples of WAN traffic kept in the browser: 15 minutes at one every 10 seconds

export interface TrafficPoint { at: number; rx: number | null; tx: number | null }

/**
 * The summaries the monitor shows, each read as often as it changes. No lists
 * are fetched: only counts for the charts. What a
 * person may not see is simply not asked for.
 */
export function useMonitorData() {
  const canNetwork = useCan('inventory:view')
  const canLinks = useCan('links:view')
  const dashboard = useQuery({ queryKey: ['/dashboard', 'monitor'], queryFn: () => get<Dashboard>('/dashboard'), refetchInterval: 30_000 })
  const segments = useQuery({ queryKey: ['/segments', 'monitor'], queryFn: () => get<Segment[]>('/segments'), refetchInterval: 30_000, enabled: canNetwork })
  const links = useQuery({ queryKey: ['/network-links', 'dashboard', 'monitor'], queryFn: () => get<LinkDashboard>('/network-links/dashboard'), refetchInterval: 60_000, enabled: canLinks })
  const firewall = useFirewallLive(canLinks)
  const linkSettings = useLinkSettings(canLinks)
  const [traffic, setTraffic] = useState<TrafficPoint[]>([])
  const sample = firewall.data
  useEffect(() => {
    if (!sample) return
    const wan = sample.interfaces.filter((i) => i.role === 'wan' || i.link)
    const sum = (k: 'rxBps' | 'txBps') => (wan.some((i) => i[k] != null) ? wan.reduce((n, i) => n + (i[k] ?? 0), 0) : null)
    const at = Date.parse(sample.at)
    // A new sample from the firewall adds one point; the same one is ignored.
    setTraffic((t) => (t.length && t[t.length - 1].at === at ? t : [...t, { at, rx: sum('rxBps'), tx: sum('txBps') }].slice(-history)))
  }, [sample])
  return { dashboard, segments, links, linkSettings, firewall, traffic, canNetwork, canLinks }
}

/** The time in Sri Lanka, ticking every second. */
export function useClock() {
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    const t = setInterval(() => setNow(new Date()), 1000)
    return () => clearInterval(t)
  }, [])
  return now
}
