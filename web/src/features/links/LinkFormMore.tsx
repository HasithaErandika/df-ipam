import { Plus, X } from 'lucide-react'
import { Button, Field, MultiSearchSelect, Notice, SearchSelect, Select, Switch } from '../../components/ui'
import type { LinkOptions } from '../../lib/types'
import { formatMoney, relationKinds, vpnTypes } from './labels'
import { AreaField, ChoiceField, FormBlock, type SectionProps, TextField } from './formParts'

const opts = (list: { id: string; label: string; detail: string }[] = []) => list.map((o) => ({ value: o.id, label: o.label, detail: o.detail }))

/** Section C, for VPN link types only. Secrets never go here. */
export function VpnSection({ p, options }: { p: SectionProps; options?: LinkOptions }) {
  const { f, set, errors } = p
  return (
    <FormBlock title="VPN information" hint="Never enter pre-shared keys or passwords. Say where they are kept in Credential vault reference.">
      <TextField p={p} k="vpnTunnel" label="VPN tunnel name" />
      <ChoiceField p={p} k="vpnType" label="VPN type" choices={vpnTypes} emptyLabel="Not set" />
      <TextField p={p} k="lastTestDate" label="Last connectivity test" type="date" />
      <TextField p={p} k="localPeer" label="Local peer IP / FQDN" mono />
      <TextField p={p} k="remotePeer" label="Remote peer IP / FQDN" mono />
      <TextField p={p} k="vaultRef" label="Credential vault reference" placeholder="Where the key is kept, not the key" />
      <div className="sm:col-span-2 lg:col-span-3">
        <Field label="Local subnets" htmlFor="link-localSegmentIds" error={errors.localSegmentIds}>
          <MultiSearchSelect id="link-localSegmentIds" values={f.localSegmentIds} onChange={(v) => set('localSegmentIds', v)} options={opts(options?.segments)} placeholder="Add a subnet from the register…" />
        </Field>
      </div>
      <AreaField p={p} k="remoteSubnets" label="Remote subnets" mono placeholder={'10.20.0.0/16\n172.16.5.0/24'}
        hint="One per line. The far side's networks: they may overlap other partners' and are never counted as your address space." />
      <TextField p={p} k="remoteOrg" label="Remote organisation" />
      <TextField p={p} k="remoteContact" label="Remote technical contact" wide />
    </FormBlock>
  )
}

/** Section D. Hidden from people who may not manage costs; their edits keep it as it is. */
export function CommercialSection({ p, canManage, canView }: { p: SectionProps; canManage: boolean; canView: boolean }) {
  const { f, set, errors } = p
  if (!canManage) {
    return <Notice>{canView ? 'Commercial details can be changed by people who manage link costs. Saving keeps them as they are.' : 'Commercial details are kept by people who manage link costs.'}</Notice>
  }
  const rental = Number(f.monthlyRental.replace(/,/g, ''))
  return (
    <FormBlock title="Commercial information" hint="All optional: not every tunnel or internal link has its own charge.">
      <div className="sm:col-span-2 lg:col-span-3">
        <Switch id="link-noSeparateCost" checked={f.noSeparateCost} onChange={(v) => set('noSeparateCost', v)} label="No separate recurring cost"
          hint="For a link paid for through another, such as a VPN over an internet line, so its cost isn't counted twice." />
      </div>
      {!f.noSeparateCost && <>
        <TextField p={p} k="monthlyRental" label="Monthly rental" />
        <Field label="Currency" htmlFor="link-currency" error={errors.currency}>
          <Select id="link-currency" value={['LKR', 'USD'].includes(f.currency) ? f.currency : 'other'} onChange={(e) => set('currency', e.target.value === 'other' ? '' : e.target.value)}>
            <option value="LKR">LKR</option><option value="USD">USD</option><option value="other">Other</option>
          </Select>
        </Field>
        {!['LKR', 'USD'].includes(f.currency) && <TextField p={p} k="currency" label="Currency code" placeholder="EUR" />}
        <Field label="Annual recurring cost" htmlFor="link-annual" hint="Monthly rental × 12, worked out for you.">
          <div id="link-annual" className="flex h-8 items-center text-[13px] text-ink tabular-nums">{f.monthlyRental && Number.isFinite(rental) ? formatMoney(rental * 12, f.currency || 'LKR') : '—'}</div>
        </Field>
      </>}
      <TextField p={p} k="installCost" label="One-time installation cost" />
      <TextField p={p} k="billingAccount" label="Billing account number" />
      <TextField p={p} k="contractRef" label="Contract reference" />
      <TextField p={p} k="poRef" label="Purchase order reference" />
      <TextField p={p} k="costCentre" label="Cost centre" />
      <TextField p={p} k="activationDate" label="Activation date" type="date" />
      <TextField p={p} k="contractStart" label="Contract start" type="date" />
      <TextField p={p} k="contractEnd" label="Contract expiry" type="date" hint="Reminders go out 90, 60, 30 and 7 days before, and on the day." />
      <TextField p={p} k="renewalNoticeDays" label="Renewal notice period (days)" />
    </FormBlock>
  )
}

/** Primary, secondary, backup and underlying links. */
export function RelationsSection({ p, options, selfId }: { p: SectionProps; options?: LinkOptions; selfId?: string }) {
  const { f, set, errors } = p
  const used = new Set(f.relations.map((r) => r.relatedId))
  const choices = opts(options?.links).filter((o) => o.value !== selfId)
  const change = (i: number, patch: Partial<{ relatedId: string; kind: string }>) => set('relations', f.relations.map((r, j) => (i === j ? { ...r, ...patch } : r)))
  return (
    <section aria-label="Related links" className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-[12.5px] text-ink-3">This link's secondary and backup links, and for a VPN the internet line it runs over.</p>
        <Button onClick={() => set('relations', [...f.relations, { relatedId: '', kind: 'backup' }])}><Plus size={14} aria-hidden />Add related link</Button>
      </div>
      <div className="space-y-2">
        {f.relations.length === 0 && <p className="rounded-md border border-dashed border-line px-3 py-6 text-center text-[13px] text-ink-3">No related links.</p>}
        {f.relations.map((r, i) => (
          <div key={i} className="grid gap-2 sm:grid-cols-[1fr_180px_auto]">
            <SearchSelect id={`link-rel-${i}`} value={r.relatedId} allowEmpty={false} placeholder="Choose a link…"
              options={choices.filter((o) => o.value === r.relatedId || !used.has(o.value))} onChange={(v) => change(i, { relatedId: v })} />
            <Select aria-label="Relationship" value={r.kind} onChange={(e) => change(i, { kind: e.target.value })}>
              {Object.entries(relationKinds).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
            </Select>
            <Button variant="ghost" aria-label="Remove" onClick={() => set('relations', f.relations.filter((_, j) => j !== i))}><X size={15} /></Button>
          </div>
        ))}
        {errors.relations && <p role="alert" className="text-[13px] text-danger">{errors.relations}</p>}
      </div>
    </section>
  )
}
