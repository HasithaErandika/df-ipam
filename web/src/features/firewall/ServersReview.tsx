import { useState } from 'react'
import { Link } from 'react-router'
import { Card, EmptyState, Input, Select, StateBadge } from '../../components/ui'
import { post } from '../../lib/api'
import { ipTypes } from '../../lib/labels'
import type { FirewallInventory } from '../../lib/types'
import type { ReviewProps, SaveTask } from './FirewallImport'
import { ListTools, SaveBar } from './ImportParts'
import { hostOpen } from './reviewCounts'

type Host = FirewallInventory['servers'][number]
interface Pick { on: boolean; name: string; ipType: string; monitor: boolean }

const sourceText = { address: 'Address object', vip: 'Published (virtual IP)', reserved: 'DHCP reservation' }

function stateBadge(h: Host) {
  if (h.registered) return <StateBadge tone="ok">Registered{h.registered.name ? ` as ${h.registered.name}` : ''}</StateBadge>
  return h.segmentId ? <StateBadge tone="accent">Not registered</StateBadge> : <StateBadge tone="neutral">No segment</StateBadge>
}

/** Addresses the firewall names (servers, published hosts, reservations) as register entries. */
export function ServersReview({ inv, busy, onSave }: ReviewProps) {
  const [picks, setPicks] = useState<Record<string, Pick>>({})
  const [onlyOpen, setOnlyOpen] = useState(true)
  const pick = (h: Host): Pick => picks[h.ip] ?? { on: false, name: h.name, ipType: h.ipType, monitor: h.ipType === 'server' }
  const set = (h: Host, p: Partial<Pick>) => setPicks((all) => ({ ...all, [h.ip]: { ...pick(h), ...p } }))
  const rows = inv.servers.filter((h) => !onlyOpen || hostOpen(h))
  const openRows = rows.filter(hostOpen)
  const chosen = inv.servers.filter((h) => hostOpen(h) && pick(h).on)
  const chooseAll = (on: boolean) => setPicks((all) => ({ ...all, ...Object.fromEntries(openRows.map((h) => [h.ip, { ...pick(h), on }])) }))
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
      <ListTools all={inv.servers.length} open={inv.servers.filter(hostOpen).length} onlyOpen={onlyOpen} setOnlyOpen={setOnlyOpen}
        allChosen={openRows.length > 0 && openRows.every((h) => pick(h).on)} chooseAll={chooseAll} />
      {rows.length === 0 ? <EmptyState title="Every named host is registered" body="Choose All to see them." /> : (
        <ul className="divide-y divide-line">
          {rows.map((h) => {
            const p = pick(h)
            const id = `fw-host-${h.ip.replace(/\./g, '-')}`
            return (
              <li key={h.ip} className={`grid gap-3 px-4 py-3 lg:grid-cols-[auto_1fr_220px_170px_auto] lg:items-center ${p.on ? 'bg-accent-soft/30' : ''}`}>
                <input id={id} type="checkbox" className="size-4 accent-[var(--accent)]" disabled={!hostOpen(h)} checked={p.on} onChange={(e) => set(h, { on: e.target.checked })} />
                <label htmlFor={id} className="min-w-0 cursor-pointer text-[13px]">
                  <span className="flex flex-wrap items-center gap-2">
                    <span className="font-mono font-medium text-ink">{h.ip}</span>
                    {stateBadge(h)}
                    {h.segmentId && <Link className="text-ink-3 hover:text-accent hover:underline" to={`/segments/${h.segmentId}`}>{h.segmentName}</Link>}
                  </span>
                  <span className="block text-[12.5px] text-ink-3">{h.sources.map((s) => `${sourceText[s.kind]}: ${s.name}${s.comment && s.comment !== s.name ? ` (${s.comment})` : ''}`).join(' · ')}{h.mac && <span className="font-mono"> · {h.mac}</span>}</span>
                  {h.notes.map((x) => <span key={x} className="block text-[12px] text-warn">{x}</span>)}
                </label>
                {hostOpen(h) ? <>
                  <Input aria-label={`Assigned to, for ${h.ip}`} value={p.name} placeholder="Assigned to" onChange={(e) => set(h, { name: e.target.value })} />
                  <Select aria-label={`IP type for ${h.ip}`} value={p.ipType} onChange={(e) => set(h, { ipType: e.target.value })}>{Object.entries(ipTypes).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</Select>
                  <label className="flex items-center gap-1.5 text-[12.5px] text-ink-2"><input type="checkbox" className="size-4 accent-[var(--accent)]" checked={p.monitor} onChange={(e) => set(h, { monitor: e.target.checked })} />Monitor</label>
                </> : null}
              </li>
            )
          })}
        </ul>
      )}
      <SaveBar count={chosen.length} noun={chosen.length === 1 ? 'address' : 'addresses'} busy={busy}
        onClear={() => setPicks({})} onSave={() => onSave(tasks(), 'register entries')} />
    </Card>
  )
}
