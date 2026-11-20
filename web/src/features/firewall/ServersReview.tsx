import { useState } from 'react'
import { Link } from 'react-router'
import { Button, Card, EmptyState, Input, Select, StateBadge } from '../../components/ui'
import { post } from '../../lib/api'
import { ipTypes } from '../../lib/labels'
import type { FirewallInventory } from '../../lib/types'
import type { ReviewProps, SaveTask } from './InventoryReview'

type Host = FirewallInventory['servers'][number]
interface Pick { on: boolean; name: string; ipType: string; monitor: boolean }

const sourceText = { address: 'Address object', vip: 'Published (virtual IP)', reserved: 'DHCP reservation' }

/** Addresses the firewall names (servers, published hosts, reservations) as register entries. */
export function ServersReview({ inv, busy, onSave }: ReviewProps) {
  const [picks, setPicks] = useState<Record<string, Pick>>({})
  const pick = (h: Host): Pick => picks[h.ip] ?? { on: false, name: h.name, ipType: h.ipType, monitor: h.ipType === 'server' }
  const set = (h: Host, p: Partial<Pick>) => setPicks((all) => ({ ...all, [h.ip]: { ...pick(h), ...p } }))
  const can = (h: Host) => !h.registered && Boolean(h.segmentId)
  const chosen = inv.servers.filter((h) => can(h) && pick(h).on)
  const tasks = (): SaveTask[] => chosen.map((h) => {
    const p = pick(h)
    return { label: `Register ${h.ip} as ${p.name || 'unnamed'} in ${h.segmentName}`, run: () => post('/ip-addresses', {
      segmentId: h.segmentId, ip: h.ip, status: 'assigned', ipType: p.ipType, assignedName: p.name.trim(), expectedMac: h.mac,
      platform: '', services: [], owner: '', department: '', monitor: p.monitor, critical: false,
      notes: `From the FortiGate: ${h.sources.map((s) => `${sourceText[s.kind].toLowerCase()} ${s.name}`).join(', ')}.`,
    }) }
  })
  if (inv.servers.length === 0) return <Card><EmptyState title="The firewall names no hosts" body="It has no single-host address objects, virtual IPs or DHCP reservations." /></Card>
  return (
    <Card title="Servers and named hosts" subtitle="Single-host address objects, servers published through virtual IPs, and DHCP reservations. Register the ones you keep.">
      <ul className="divide-y divide-line">
        {inv.servers.map((h) => {
          const p = pick(h)
          return (
            <li key={h.ip} className={`grid gap-3 px-4 py-3 lg:grid-cols-[auto_1fr_220px_170px_auto] lg:items-center ${p.on ? 'bg-accent-soft/30' : ''}`}>
              <input type="checkbox" aria-label={`Choose ${h.ip}`} className="size-4 accent-[var(--accent)]" disabled={!can(h)} checked={p.on} onChange={(e) => set(h, { on: e.target.checked })} />
              <div className="min-w-0 text-[13px]">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-mono font-medium text-ink">{h.ip}</span>
                  {h.registered ? <StateBadge tone="ok">Registered{h.registered.name ? ` as ${h.registered.name}` : ''}</StateBadge> : h.segmentId ? <StateBadge tone="accent">Not registered</StateBadge> : <StateBadge tone="neutral">No segment</StateBadge>}
                  {h.segmentId && <Link className="text-ink-3 hover:text-accent hover:underline" to={`/segments/${h.segmentId}`}>{h.segmentName}</Link>}
                </div>
                <div className="text-[12px] text-ink-2">{h.sources.map((s) => `${sourceText[s.kind]}: ${s.name}${s.comment && s.comment !== s.name ? ` (${s.comment})` : ''}`).join(' · ')}{h.mac && <span className="font-mono"> · {h.mac}</span>}</div>
                {h.notes.map((x) => <div key={x} className="text-[12px] text-warn">{x}</div>)}
              </div>
              {can(h) ? <>
                <Input aria-label="Assigned to" value={p.name} onChange={(e) => set(h, { name: e.target.value })} />
                <Select aria-label="IP type" value={p.ipType} onChange={(e) => set(h, { ipType: e.target.value })}>{Object.entries(ipTypes).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</Select>
                <label className="flex items-center gap-1.5 text-[12.5px] text-ink-2"><input type="checkbox" className="size-4 accent-[var(--accent)]" checked={p.monitor} onChange={(e) => set(h, { monitor: e.target.checked })} />Monitor</label>
              </> : <><span /><span /><span /></>}
            </li>
          )
        })}
      </ul>
      <div className="flex items-center justify-end gap-3 border-t border-line px-4 py-3">
        <span className="mr-auto text-[13px] text-ink-2">{chosen.length} chosen to register</span>
        <Button variant="primary" busy={busy} disabled={chosen.length === 0} onClick={() => onSave(tasks(), 'register entries')}>Review and save</Button>
      </div>
    </Card>
  )
}
