import { useState } from 'react'
import { Link } from 'react-router'
import { Button, Card, EmptyState, Input, Select, StateBadge } from '../../components/ui'
import { post, put } from '../../lib/api'
import { segmentKinds } from '../../lib/labels'
import { useSegments } from '../../lib/queries'
import type { FirewallInventory } from '../../lib/types'
import type { ReviewProps, SaveTask } from './InventoryReview'
import { shown } from './format'

type Net = FirewallInventory['networks'][number]
interface Pick { on: boolean; name: string; kind: string }

const fieldNames: Record<string, string> = { gateway: 'Gateway', vlan: 'VLAN', dhcpPool: 'DHCP pool' }

/** Firewall interface networks and VPN pools, as segments. */
export function NetworksReview({ inv, busy, onSave }: ReviewProps) {
  const segments = useSegments()
  const [picks, setPicks] = useState<Record<string, Pick>>({})
  const pick = (n: Net): Pick => picks[n.key] ?? { on: false, name: n.name, kind: n.kind }
  const set = (n: Net, p: Partial<Pick>) => setPicks((all) => ({ ...all, [n.key]: { ...pick(n), ...p } }))
  const usable = (n: Net) => n.canAdd || (n.existing && n.changes.length > 0)
  const chosen = inv.networks.filter((n) => usable(n) && pick(n).on)
  const tasks = (): SaveTask[] => chosen.map((n) => {
    const p = pick(n)
    if (!n.existing) {
      return { label: `Add segment ${p.name} (${n.network})`, run: () => post('/segments', {
        name: p.name, kind: p.kind, vlan: n.vlan, network: n.network, gateway: p.kind === 'vpn' ? '' : n.gateway,
        dhcpStart: p.kind === 'dhcp' ? n.dhcpStart : '', dhcpEnd: p.kind === 'dhcp' ? n.dhcpEnd : '',
        description: `From the FortiGate (${n.interface}).`, alertPrivateMacs: false, checkPorts: false,
      }) }
    }
    const s = segments.data?.find((x) => x.id === n.existing!.id)
    return { label: `Update ${n.existing.name}: ${n.changes.map((c) => fieldNames[c.field] ?? c.field).join(', ')}`, run: () => {
      if (!s) return Promise.reject(new Error('The segment list is still loading.'))
      return put(`/segments/${s.id}`, {
        name: s.name, kind: s.kind, floor: s.floor, vlan: n.vlan ?? s.vlan, network: s.network, gateway: n.gateway || s.gateway || '',
        dhcpStart: s.kind === 'dhcp' && n.dhcpStart ? n.dhcpStart : s.dhcpStart ?? '', dhcpEnd: s.kind === 'dhcp' && n.dhcpEnd ? n.dhcpEnd : s.dhcpEnd ?? '',
        defaultEnvironmentId: s.defaultEnvironmentId ?? '', description: s.description, alertPrivateMacs: s.alertPrivateMacs, checkPorts: s.checkPorts,
      })
    } }
  })
  if (inv.networks.length === 0) return <Card><EmptyState title="No networks to propose" body="The firewall has no LAN, DMZ or VLAN interfaces with a static address, and no VPN pools." /></Card>
  return (
    <Card title="Networks" subtitle="LAN, DMZ and VLAN interfaces, and VPN address pools. WAN interfaces are network links, on their own tab.">
      <ul className="divide-y divide-line">
        {inv.networks.map((n) => {
          const p = pick(n)
          return (
            <li key={n.key} className={`grid gap-3 px-4 py-3 lg:grid-cols-[auto_1fr_200px_160px] lg:items-center ${p.on ? 'bg-accent-soft/30' : ''}`}>
              <input type="checkbox" aria-label={`Choose ${n.name}`} className="size-4 accent-[var(--accent)]" disabled={!usable(n)} checked={p.on} onChange={(e) => set(n, { on: e.target.checked })} />
              <div className="min-w-0 text-[13px]">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-mono font-medium text-ink">{n.network}</span>
                  <span className="text-ink-3">{n.interface}{n.role ? ` · ${n.role.toUpperCase()}` : ''}</span>
                  {n.existing ? (n.changes.length ? <StateBadge tone="warn">{n.changes.length} difference{n.changes.length === 1 ? '' : 's'}</StateBadge> : <StateBadge tone="ok">Up to date</StateBadge>)
                    : n.canAdd ? <StateBadge tone="accent">New</StateBadge> : <StateBadge tone="neutral">Can't add</StateBadge>}
                  {n.existing && <Link to={`/segments/${n.existing.id}`} className="text-accent hover:underline">{n.existing.name}</Link>}
                </div>
                <div className="text-ink-2">{[n.gateway && `Gateway ${n.gateway}`, n.vlan && `VLAN ${n.vlan}`, n.dhcpStart && `DHCP ${n.dhcpStart} – ${n.dhcpEnd}`].filter(Boolean).join(' · ')}</div>
                {n.changes.map((c) => <div key={c.field} className="text-[12.5px] text-ink-2">{fieldNames[c.field]}: <span className="text-ink-3 line-through">{shown(c.old)}</span> → <strong>{shown(c.new)}</strong></div>)}
                {n.notes.map((x) => <div key={x} className="text-[12px] text-warn">{x}</div>)}
              </div>
              {!n.existing ? <Input aria-label="Segment name" value={p.name} disabled={!n.canAdd} onChange={(e) => set(n, { name: e.target.value })} /> : <span />}
              {!n.existing ? (
                <Select aria-label="Segment type" value={p.kind} disabled={!n.canAdd} onChange={(e) => set(n, { kind: e.target.value })}>
                  {Object.entries(segmentKinds).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
                </Select>
              ) : <span />}
            </li>
          )
        })}
      </ul>
      <div className="flex items-center justify-end gap-3 border-t border-line px-4 py-3">
        <span className="mr-auto text-[13px] text-ink-2">{chosen.length} chosen</span>
        <Button variant="primary" busy={busy} disabled={chosen.length === 0 || chosen.some((n) => !n.existing && !pick(n).name.trim())} onClick={() => onSave(tasks(), 'network changes')}>Review and save</Button>
      </div>
    </Card>
  )
}
