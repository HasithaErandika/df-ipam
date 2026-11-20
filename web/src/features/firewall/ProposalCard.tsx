import { Link } from 'react-router'
import { Input, SearchSelect, Select, StateBadge } from '../../components/ui'
import type { FirewallProposal, LinkProvider, LinkSettings } from '../../lib/types'
import { criticalities, linkRoles } from '../links/labels'
import { firewallFields, shown } from './format'

/** What people decide about one proposal. */
export interface Choice {
  picked: boolean
  name: string; linkType: string; sourceSite: string; purpose: string; linkRole: string; criticality: string; providerId: string
  fields: string[]
}

export const startChoice = (p: FirewallProposal, settings?: LinkSettings): Choice => ({
  picked: false, name: p.suggestion.name, linkType: p.suggestion.linkType, sourceSite: settings?.sites.length === 1 ? settings.sites[0] : '',
  purpose: '', linkRole: p.kind === 'interface' ? 'primary' : 'standalone', criticality: 'high', providerId: '', fields: p.changes.map((c) => c.field),
})

/** What is missing before a new link can be added. */
export function missing(c: Choice): string[] {
  const out: string[] = []
  if (!c.name.trim()) out.push('name')
  if (!c.linkType) out.push('type')
  if (!c.sourceSite) out.push('location')
  if (!c.purpose.trim()) out.push('purpose')
  return out
}

const kinds = { interface: 'WAN interface', ipsec: 'Site-to-site tunnel', dialup: 'Dial-up VPN' }

/** One interface or tunnel from the firewall: its facts, and the choice to add it or update its link. */
export function ProposalCard({ p, c, onChange, settings, providers }: {
  p: FirewallProposal; c: Choice; onChange: (c: Choice) => void; settings?: LinkSettings; providers: LinkProvider[]
}) {
  const set = (patch: Partial<Choice>) => onChange({ ...c, ...patch })
  const upToDate = p.existing && p.changes.length === 0
  const id = `fw-${p.key.replace(/[^a-z0-9]/gi, '-')}`
  return (
    <li className={`rounded-md border bg-surface ${c.picked ? 'border-accent/60 ring-1 ring-accent/20' : 'border-line'}`}>
      <label className="flex cursor-pointer items-start gap-3 px-4 py-3" htmlFor={id}>
        <input id={id} type="checkbox" className="mt-1 size-4 accent-[var(--accent)]" checked={c.picked} disabled={Boolean(upToDate)} onChange={(e) => set({ picked: e.target.checked })} />
        <span className="min-w-0 flex-1">
          <span className="flex flex-wrap items-center gap-2">
            <span className="font-medium text-ink">{p.title}</span>
            <span className="text-[12px] text-ink-3">{kinds[p.kind]}</span>
            {!p.existing && <StateBadge tone="accent">New</StateBadge>}
            {p.existing && (upToDate ? <StateBadge tone="ok">Recorded, up to date</StateBadge> : <StateBadge tone="warn">{p.changes.length} difference{p.changes.length === 1 ? '' : 's'}</StateBadge>)}
            {p.existing && <Link to={`/network-links/${p.existing.id}`} className="text-[12.5px] text-accent hover:underline" onClick={(e) => e.stopPropagation()}>{p.existing.code}</Link>}
          </span>
          <span className="mt-1 flex flex-wrap gap-x-4 gap-y-0.5 text-[12.5px] text-ink-2">
            {p.facts.map((f) => <span key={f.label}><span className="text-ink-3">{f.label}:</span> {f.value}</span>)}
          </span>
          {p.notes.map((n) => <span key={n} className="mt-1 block text-[12px] text-warn">{n}</span>)}
        </span>
      </label>
      {c.picked && !p.existing && <AddFields p={p} c={c} set={set} settings={settings} providers={providers} />}
      {c.picked && p.existing && !upToDate && (
        <div className="border-t border-line px-4 py-3">
          <p className="mb-2 text-[12.5px] text-ink-2">Update {p.existing.code} with what the firewall has now:</p>
          <ul className="space-y-1">
            {p.changes.map((ch) => (
              <li key={ch.field}>
                <label className="flex cursor-pointer items-center gap-2 text-[13px]">
                  <input type="checkbox" className="size-4 accent-[var(--accent)]" checked={c.fields.includes(ch.field)}
                    onChange={(e) => set({ fields: e.target.checked ? [...c.fields, ch.field] : c.fields.filter((f) => f !== ch.field) })} />
                  {firewallFields[ch.field] ?? ch.field}: <span className="text-ink-3 line-through">{shown(ch.old)}</span> → <span className="font-medium">{shown(ch.new)}</span>
                </label>
              </li>
            ))}
          </ul>
        </div>
      )}
    </li>
  )
}

function AddFields({ p, c, set, settings, providers }: { p: FirewallProposal; c: Choice; set: (x: Partial<Choice>) => void; settings?: LinkSettings; providers: LinkProvider[] }) {
  const id = (k: string) => `fw-${k}-${p.key.replace(/[^a-z0-9]/gi, '-')}`
  const label = (k: string, text: string, el: React.ReactNode) => <label htmlFor={id(k)} className="space-y-1"><span className="block text-[12.5px] font-medium text-ink">{text}</span>{el}</label>
  const lacks = missing(c)
  return (
    <div className="grid gap-3 border-t border-line px-4 py-3 sm:grid-cols-2 lg:grid-cols-3">
      {label('name', 'Link name *', <Input id={id('name')} value={c.name} onChange={(e) => set({ name: e.target.value })} />)}
      {label('type', 'Link type *', <Select id={id('type')} value={c.linkType} onChange={(e) => set({ linkType: e.target.value })}>
        {(settings?.types ?? []).filter((t) => t.active).map((t) => <option key={t.key} value={t.key}>{t.label}</option>)}</Select>)}
      {label('site', 'Source location *', <Select id={id('site')} value={c.sourceSite} onChange={(e) => set({ sourceSite: e.target.value })}>
        <option value="">Choose…</option>{settings?.sites.map((s) => <option key={s}>{s}</option>)}</Select>)}
      <div className="sm:col-span-2">{label('purpose', 'Business purpose *', <Input id={id('purpose')} value={c.purpose} placeholder="What the business uses it for" onChange={(e) => set({ purpose: e.target.value })} />)}</div>
      {label('provider', 'Service provider', <SearchSelect id={id('provider')} value={c.providerId} onChange={(v) => set({ providerId: v })} emptyLabel="No provider"
        options={providers.filter((x) => x.active).map((x) => ({ value: x.id, label: x.name }))} />)}
      {label('role', 'Link role *', <Select id={id('role')} value={c.linkRole} onChange={(e) => set({ linkRole: e.target.value })}>
        {Object.entries(linkRoles).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</Select>)}
      {label('crit', 'Criticality *', <Select id={id('crit')} value={c.criticality} onChange={(e) => set({ criticality: e.target.value })}>
        {Object.entries(criticalities).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</Select>)}
      {lacks.length > 0 && <p className="self-end text-[12.5px] text-warn sm:col-span-2 lg:col-span-3">Still needed: {lacks.join(', ')}.</p>}
    </div>
  )
}
