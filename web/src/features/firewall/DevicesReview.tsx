import { useState } from 'react'
import { Button, Card, EmptyState, FilterBar, FilterSelect, Input, SearchInput, Select, StateBadge } from '../../components/ui'
import { post } from '../../lib/api'
import { deviceTypes } from '../../lib/labels'
import { platforms } from '../../lib/platforms'
import type { FirewallInventory } from '../../lib/types'
import type { ReviewProps, SaveTask } from './InventoryReview'

type Dev = FirewallInventory['devices'][number]
interface Pick { on: boolean; name: string; deviceType: string; platform: string }

const statusTone = { none: 'accent', pending: 'warn', approved: 'ok', rejected: 'danger' } as const
const statusText = { none: 'Not in DF-IPAM', pending: 'Waiting for a decision', approved: 'Approved', rejected: 'Rejected' }
const shownMax = 300

/** Every MAC the firewall knows, with its name and system; approve the ones that belong. */
export function DevicesReview({ inv, busy, onSave }: ReviewProps) {
  const [picks, setPicks] = useState<Record<string, Pick>>({})
  const [q, setQ] = useState('')
  const [which, setWhich] = useState('open')
  const pick = (d: Dev): Pick => picks[d.mac] ?? { on: false, name: d.hostname || d.deviceName, deviceType: d.deviceType, platform: d.platform }
  const set = (d: Dev, p: Partial<Pick>) => setPicks((all) => ({ ...all, [d.mac]: { ...pick(d), ...p } }))
  const needle = q.trim().toLowerCase()
  const rows = inv.devices.filter((d) => (which === 'all' || d.deviceStatus !== 'approved') && (!needle || [d.mac, d.ip, d.hostname, d.os, d.vendor].some((v) => v.toLowerCase().includes(needle))))
  const open = (d: Dev) => d.deviceStatus !== 'approved'
  const chosen = inv.devices.filter((d) => open(d) && pick(d).on)
  const visibleOpen = rows.filter(open).slice(0, shownMax)
  const allOn = visibleOpen.length > 0 && visibleOpen.every((d) => pick(d).on)
  const tasks = (): SaveTask[] => chosen.map((d) => {
    const p = pick(d)
    const body = { name: p.name.trim() || d.mac, deviceType: p.deviceType, platform: p.platform, owner: '', department: '' }
    return d.deviceId
      ? { label: `Approve ${body.name} (${d.mac})`, run: () => post(`/devices/${d.deviceId}/decision`, { status: 'approved', ...body }) }
      : { label: `Add and approve ${body.name} (${d.mac})`, run: () => post('/devices', { mac: d.mac, ...body, notes: `From the FortiGate${d.os ? `: ${d.os}` : ''}.` }) }
  })
  if (inv.devices.length === 0) return <Card><EmptyState title="The firewall knows no devices" body="Its address table, DHCP leases and device identification are empty." /></Card>
  return (
    <Card title="Devices" subtitle="From the firewall's address table, DHCP leases and device identification. Approve the ones that belong; the rest stay unknown and keep raising alerts.">
      <FilterBar>
        <SearchInput value={q} onChange={setQ} placeholder="Search MAC, IP, name, system" />
        <FilterSelect label="Show" value={which === 'all' ? 'all' : ''} anyLabel="Not approved yet" onChange={(v) => setWhich(v || 'open')} options={[{ value: 'all', text: 'All devices' }]} />
        <label className="ml-auto flex items-center gap-2 text-[13px] text-ink-2">
          <input type="checkbox" className="size-4 accent-[var(--accent)]" checked={allOn} onChange={(e) => setPicks((all) => ({ ...all, ...Object.fromEntries(visibleOpen.map((d) => [d.mac, { ...pick(d), on: e.target.checked }])) }))} />
          Choose all shown
        </label>
      </FilterBar>
      <div className="overflow-x-auto">
        <table className="w-full text-[13px]">
          <thead><tr className="bg-surface-2 text-left text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase">
            <th className="w-8 px-3 py-2" /><th className="px-3 py-2">Device on the firewall</th><th className="px-3 py-2">In DF-IPAM</th><th className="px-3 py-2">Name</th><th className="px-3 py-2">Type</th><th className="px-3 py-2">Platform</th>
          </tr></thead>
          <tbody>
            {rows.slice(0, shownMax).map((d) => {
              const p = pick(d)
              return (
                <tr key={d.mac} className={`border-t border-line ${p.on ? 'bg-accent-soft/30' : ''}`}>
                  <td className="px-3 py-2"><input type="checkbox" aria-label={`Choose ${d.mac}`} className="size-4 accent-[var(--accent)]" disabled={!open(d)} checked={p.on} onChange={(e) => set(d, { on: e.target.checked })} /></td>
                  <td className="px-3 py-2">
                    <div className="font-mono text-[12.5px] text-ink">{d.mac}{d.privateMac && <span className="ml-1 font-sans text-[11.5px] text-ink-3">(private)</span>}</div>
                    <div className="text-[12px] text-ink-3">{[d.ip, d.hostname, d.os, d.vendor].filter(Boolean).join(' · ')}</div>
                  </td>
                  <td className="px-3 py-2"><StateBadge tone={statusTone[d.deviceStatus]}>{statusText[d.deviceStatus]}</StateBadge></td>
                  <td className="px-3 py-2"><Input aria-label="Name" className="min-w-36" value={p.name} disabled={!open(d)} placeholder={d.mac} onChange={(e) => set(d, { name: e.target.value })} /></td>
                  <td className="px-3 py-2"><Select aria-label="Type" value={p.deviceType} disabled={!open(d)} onChange={(e) => set(d, { deviceType: e.target.value })}>{Object.entries(deviceTypes).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</Select></td>
                  <td className="px-3 py-2"><Select aria-label="Platform" value={p.platform} disabled={!open(d)} onChange={(e) => set(d, { platform: e.target.value })}><option value="">Not set</option>{Object.entries(platforms).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</Select></td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      {rows.length > shownMax && <p className="px-4 py-2 text-[12.5px] text-ink-3">Showing {shownMax} of {rows.length}. Search to narrow the list.</p>}
      <div className="flex items-center justify-end gap-3 border-t border-line px-4 py-3">
        <span className="mr-auto text-[13px] text-ink-2">{chosen.length} chosen to approve</span>
        <Button variant="primary" busy={busy} disabled={chosen.length === 0} onClick={() => onSave(tasks(), 'devices')}>Review and save</Button>
      </div>
    </Card>
  )
}
