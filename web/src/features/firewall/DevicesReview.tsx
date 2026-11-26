import { useState } from 'react'
import { Card, EmptyState, Input, SearchInput, Select, StateBadge } from '../../components/ui'
import { post } from '../../lib/api'
import { deviceTypes } from '../../lib/labels'
import { platforms } from '../../lib/platforms'
import type { FirewallInventory } from '../../lib/types'
import type { ReviewProps, SaveTask } from './FirewallImport'
import { ListTools, SaveBar } from './ImportParts'
import { devOpen } from './reviewCounts'

type Dev = FirewallInventory['devices'][number]
interface Pick { on: boolean; name: string; deviceType: string; platform: string }

const statusTone = { none: 'accent', pending: 'warn', approved: 'ok', rejected: 'danger' } as const
const statusText = { none: 'Not in DF-Atlas', pending: 'Waiting for a decision', approved: 'Approved', rejected: 'Rejected' }
const shownMax = 300
const th = 'px-3 py-2 text-left text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase'

/** Every MAC the firewall knows, with its name and system; approve the ones that belong. */
export function DevicesReview({ inv, busy, onSave }: ReviewProps) {
  const [picks, setPicks] = useState<Record<string, Pick>>({})
  const [q, setQ] = useState('')
  const [onlyOpen, setOnlyOpen] = useState(true)
  const pick = (d: Dev): Pick => picks[d.mac] ?? { on: false, name: d.hostname || d.deviceName, deviceType: d.deviceType, platform: d.platform }
  const set = (d: Dev, p: Partial<Pick>) => setPicks((all) => ({ ...all, [d.mac]: { ...pick(d), ...p } }))
  const needle = q.trim().toLowerCase()
  const rows = inv.devices.filter((d) => (!onlyOpen || devOpen(d)) && (!needle || [d.mac, d.ip, d.hostname, d.os, d.vendor].some((v) => v.toLowerCase().includes(needle))))
  const visible = rows.slice(0, shownMax)
  const openVisible = visible.filter(devOpen)
  const chosen = inv.devices.filter((d) => devOpen(d) && pick(d).on)
  const chooseAll = (on: boolean) => setPicks((all) => ({ ...all, ...Object.fromEntries(openVisible.map((d) => [d.mac, { ...pick(d), on }])) }))
  const tasks = (): SaveTask[] => chosen.map((d) => {
    const p = pick(d)
    const body = { name: p.name.trim() || d.mac, deviceType: p.deviceType, platform: p.platform }
    // Approving leaves a known device's owner and department as they are.
    return d.deviceId
      ? { label: `Approve ${body.name} (${d.mac})`, run: () => post(`/devices/${d.deviceId}/decision`, { status: 'approved', ...body }) }
      : { label: `Add and approve ${body.name} (${d.mac})`, run: () => post('/devices', { mac: d.mac, ...body, owner: '', department: '', notes: `From the FortiGate${d.os ? `: ${d.os}` : ''}.` }) }
  })
  if (inv.devices.length === 0) return <Card><EmptyState title="The firewall knows no devices" body="Its address table, DHCP leases and device identification are empty." /></Card>
  return (
    <Card title="Devices" subtitle="From the firewall's address table, DHCP leases and device identification. Approve the ones that belong; the rest stay unknown and keep raising alerts.">
      <ListTools all={inv.devices.length} open={inv.devices.filter(devOpen).length} onlyOpen={onlyOpen} setOnlyOpen={setOnlyOpen}
        allChosen={openVisible.length > 0 && openVisible.every((d) => pick(d).on)} chooseAll={chooseAll}>
        <div className="w-full max-w-xs"><SearchInput value={q} onChange={setQ} placeholder="Search MAC, IP, name, system" /></div>
      </ListTools>
      {visible.length === 0 ? <EmptyState title={needle ? 'Nothing matches the search' : 'Every device is approved'} body={needle ? 'Change the search to see more.' : 'Choose All to see them.'} /> : (
        <table className="w-full table-fixed text-[13px]">
          <thead className="bg-surface-2"><tr>
            <th className="w-10 px-3 py-2" /><th className={th}>On the firewall</th><th className={`${th} w-[17%] max-md:hidden`}>In DF-Atlas</th>
            <th className={`${th} w-[20%]`}>Name</th><th className={`${th} w-[15%] max-lg:hidden`}>Type</th><th className={`${th} w-[15%] max-lg:hidden`}>Platform</th>
          </tr></thead>
          <tbody>
            {visible.map((d) => {
              const p = pick(d)
              const off = !devOpen(d)
              return (
                <tr key={d.mac} className={`border-t border-line align-middle ${p.on ? 'bg-accent-soft/30' : ''}`}>
                  <td className="px-3 py-2"><input type="checkbox" aria-label={`Choose ${d.mac}`} className="size-4 accent-[var(--accent)]" disabled={off} checked={p.on} onChange={(e) => set(d, { on: e.target.checked })} /></td>
                  <td className="overflow-hidden px-3 py-2">
                    <div className="truncate font-mono text-[12.5px] text-ink">{d.mac}{d.privateMac && <span className="ml-1 font-sans text-[11.5px] text-ink-3">(private)</span>}</div>
                    <div className="truncate text-[12px] text-ink-3" title={[d.ip, d.hostname, d.os, d.vendor].filter(Boolean).join(' · ')}>{[d.ip, d.hostname, d.os, d.vendor].filter(Boolean).join(' · ')}</div>
                  </td>
                  <td className="overflow-hidden px-3 py-2 max-md:hidden"><StateBadge tone={statusTone[d.deviceStatus]}>{statusText[d.deviceStatus]}</StateBadge></td>
                  <td className="px-3 py-2"><Input aria-label={`Name for ${d.mac}`} value={p.name} disabled={off} placeholder={d.mac} onChange={(e) => set(d, { name: e.target.value })} /></td>
                  <td className="px-3 py-2 max-lg:hidden"><Select aria-label={`Type for ${d.mac}`} value={p.deviceType} disabled={off} onChange={(e) => set(d, { deviceType: e.target.value })}>{Object.entries(deviceTypes).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</Select></td>
                  <td className="px-3 py-2 max-lg:hidden"><Select aria-label={`Platform for ${d.mac}`} value={p.platform} disabled={off} onChange={(e) => set(d, { platform: e.target.value })}><option value="">Not set</option>{Object.entries(platforms).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</Select></td>
                </tr>
              )
            })}
          </tbody>
        </table>
      )}
      {rows.length > shownMax && <p className="border-t border-line px-4 py-2 text-[12.5px] text-ink-3">Showing {shownMax} of {rows.length}. Search to narrow the list.</p>}
      <SaveBar count={chosen.length} noun={chosen.length === 1 ? 'device' : 'devices'} busy={busy}
        onClear={() => setPicks({})} onSave={() => onSave(tasks(), 'devices')} />
    </Card>
  )
}
