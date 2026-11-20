import { useState } from 'react'
import { Link } from 'react-router'
import { Card, EmptyState, Input, Select, StateBadge } from '../../components/ui'
import { post, put } from '../../lib/api'
import { segmentKinds } from '../../lib/labels'
import { useSegments } from '../../lib/queries'
import type { FirewallInventory } from '../../lib/types'
import type { ReviewProps, SaveTask } from './FirewallImport'
import { shown } from './format'
import { ListTools, SaveBar } from './ImportParts'
import { netOpen } from './reviewCounts'

type Net = FirewallInventory['networks'][number]
interface Pick { on: boolean; name: string; kind: string }

const fieldNames: Record<string, string> = { gateway: 'Gateway', vlan: 'VLAN', dhcpPool: 'DHCP pool' }

function stateBadge(n: Net) {
  if (!n.existing) return n.canAdd ? <StateBadge tone="accent">New</StateBadge> : <StateBadge tone="neutral">Can't add</StateBadge>
  return n.changes.length ? <StateBadge tone="warn">{n.changes.length} difference{n.changes.length === 1 ? '' : 's'}</StateBadge> : <StateBadge tone="ok">Up to date</StateBadge>
}

/** Firewall interface networks and VPN pools, as segments. */
export function NetworksReview({ inv, busy, onSave }: ReviewProps) {
  const segments = useSegments()
  const [picks, setPicks] = useState<Record<string, Pick>>({})
  const [onlyOpen, setOnlyOpen] = useState(true)
  const pick = (n: Net): Pick => picks[n.key] ?? { on: false, name: n.name, kind: n.kind }
  const set = (n: Net, p: Partial<Pick>) => setPicks((all) => ({ ...all, [n.key]: { ...pick(n), ...p } }))
  const rows = inv.networks.filter((n) => !onlyOpen || netOpen(n))
  const openRows = rows.filter(netOpen)
  const chosen = inv.networks.filter((n) => netOpen(n) && pick(n).on)
  const unnamed = chosen.filter((n) => !n.existing && !pick(n).name.trim()).length
  const chooseAll = (on: boolean) => setPicks((all) => ({ ...all, ...Object.fromEntries(openRows.map((n) => [n.key, { ...pick(n), on }])) }))
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
    <Card title="Networks" subtitle="LAN, DMZ and VLAN interfaces and VPN address pools, as segments. WAN interfaces are under Network links.">
      <ListTools all={inv.networks.length} open={inv.networks.filter(netOpen).length} onlyOpen={onlyOpen} setOnlyOpen={setOnlyOpen}
        allChosen={openRows.length > 0 && openRows.every((n) => pick(n).on)} chooseAll={chooseAll} />
      {rows.length === 0 ? <EmptyState title="Every network is up to date" body="Choose All to see them." /> : (
        <ul className="divide-y divide-line">
          {rows.map((n) => {
            const p = pick(n)
            const id = `fw-net-${n.key.replace(/[^a-z0-9]/gi, '-')}`
            return (
              <li key={n.key} className={`grid gap-3 px-4 py-3 lg:grid-cols-[auto_1fr_220px_170px] lg:items-center ${p.on ? 'bg-accent-soft/30' : ''}`}>
                <input id={id} type="checkbox" className="size-4 accent-[var(--accent)]" disabled={!netOpen(n)} checked={p.on} onChange={(e) => set(n, { on: e.target.checked })} />
                <label htmlFor={id} className="min-w-0 cursor-pointer text-[13px]">
                  <span className="flex flex-wrap items-center gap-2">
                    <span className="font-mono font-medium text-ink">{n.network}</span>
                    {stateBadge(n)}
                    {n.existing && <Link to={`/segments/${n.existing.id}`} className="text-accent hover:underline">{n.existing.name}</Link>}
                  </span>
                  <span className="block text-[12.5px] text-ink-3">{[`${n.interface}${n.role ? ` (${n.role.toUpperCase()})` : ''}`, n.gateway && `gateway ${n.gateway}`, n.vlan && `VLAN ${n.vlan}`, n.dhcpStart && `DHCP ${n.dhcpStart} – ${n.dhcpEnd}`].filter(Boolean).join(' · ')}</span>
                  {n.changes.map((c) => <span key={c.field} className="block text-[12.5px] text-ink-2">{fieldNames[c.field] ?? c.field}: <span className="text-ink-3 line-through">{shown(c.old)}</span> → <strong className="font-medium text-ink">{shown(c.new)}</strong></span>)}
                  {n.notes.map((x) => <span key={x} className="block text-[12px] text-warn">{x}</span>)}
                </label>
                {!n.existing && n.canAdd ? <>
                  <Input aria-label={`Segment name for ${n.network}`} value={p.name} onChange={(e) => set(n, { name: e.target.value })} />
                  <Select aria-label={`Segment type for ${n.network}`} value={p.kind} onChange={(e) => set(n, { kind: e.target.value })}>
                    {Object.entries(segmentKinds).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
                  </Select>
                </> : null}
              </li>
            )
          })}
        </ul>
      )}
      <SaveBar count={chosen.length} noun={chosen.length === 1 ? 'network' : 'networks'} busy={busy} disabled={unnamed > 0}
        note={unnamed ? `${unnamed} new segment${unnamed === 1 ? '' : 's'} need${unnamed === 1 ? 's' : ''} a name` : undefined}
        onClear={() => setPicks({})} onSave={() => onSave(tasks(), 'network changes')} />
    </Card>
  )
}
