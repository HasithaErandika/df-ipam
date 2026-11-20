import { useMutation } from '@tanstack/react-query'
import { Plus, X } from 'lucide-react'
import { useState } from 'react'
import { Button, Card, ErrorState, Field, FormError, Input, SkeletonRows, Switch, useConfirmSave } from '../../components/ui'
import { fieldIssues, put } from '../../lib/api'
import { listChanges } from '../../lib/changes'
import { useInvalidate } from '../../lib/queries'
import type { LinkSettings, LinkType } from '../../lib/types'
import { useLinkSettings } from './queries'

/** Network setup > Network links: link types, sites, reminder days and the USD rate. */
export function LinkSettingsTab() {
  const q = useLinkSettings()
  if (q.isPending) return <Card><SkeletonRows rows={6} /></Card>
  if (q.isError) return <Card><ErrorState error={q.error} onRetry={() => void q.refetch()} /></Card>
  return <SettingsForm key={JSON.stringify(q.data)} start={q.data} />
}

const keyFrom = (label: string) => label.toLowerCase().replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '').replace(/^(\d)/, 't$1').slice(0, 40) || 'type'

function SettingsForm({ start }: { start: LinkSettings }) {
  const [types, setTypes] = useState<LinkType[]>(start.types)
  const [sites, setSites] = useState(start.sites.join('\n'))
  const [days, setDays] = useState(start.reminderDays.join(', '))
  const [rate, setRate] = useState(start.usdRate == null ? '' : String(start.usdRate))
  const [errors, setErrors] = useState<Record<string, string>>({})
  const refresh = useInvalidate('/link-settings', '/network-links')
  const confirmSave = useConfirmSave()
  const body = (): LinkSettings => ({
    types: types.map((t) => ({ ...t, label: t.label.trim() })),
    sites: sites.split('\n').map((s) => s.trim()).filter(Boolean),
    reminderDays: days.split(/[\s,;]+/).filter(Boolean).map(Number),
    usdRate: rate.trim() === '' ? null : Number(rate),
  })
  const save = useMutation({
    mutationFn: () => put('/link-settings', body()),
    meta: { inlineError: true, success: 'Network link settings saved.' },
    onSuccess: () => void refresh(),
    onError: (e) => setErrors(fieldIssues(e)),
  })
  const submit = () => {
    const b = body()
    if (b.reminderDays.some((d) => !Number.isInteger(d)) || (b.usdRate != null && !Number.isFinite(b.usdRate))) {
      setErrors({ reminderDays: 'Use whole numbers of days, such as 90, 60, 30, 7, 0.', usdRate: 'Enter a number, or leave it empty.' })
      return
    }
    const changes = listChanges({ types: start.types.map((t) => `${t.label}${t.active ? '' : ' (off)'}`), sites: start.sites, days: start.reminderDays, rate: start.usdRate },
      { types: b.types.map((t) => `${t.label}${t.active ? '' : ' (off)'}`), sites: b.sites, days: b.reminderDays, rate: b.usdRate },
      { types: 'Link types', sites: 'Sites', days: 'Reminder days', rate: 'LKR per USD' })
    void confirmSave('network link settings', () => save.mutate(), 'New choices apply to links from now on, and reminders follow the new days.', changes)
  }
  const setType = (i: number, patch: Partial<LinkType>) => setTypes(types.map((t, j) => (i === j ? { ...t, ...patch } : t)))
  return (
    <div className="grid gap-4 xl:grid-cols-[1fr_380px]">
      <Card title="Link types" subtitle="Switch a type off to stop it being chosen for new links. A type that links use can't be removed. VPN types show the VPN section."
        action={<Button onClick={() => setTypes([...types, { key: '', label: '', vpn: false, active: true }])}><Plus size={14} aria-hidden />Add type</Button>}>
        <ul className="divide-y divide-line">
          {types.map((t, i) => (
            <li key={i} className="flex flex-wrap items-center gap-3 px-4 py-2">
              <Input aria-label="Type name" className="max-w-xs flex-1" value={t.label} onChange={(e) => setType(i, { label: e.target.value, key: start.types.some((s) => s.key === t.key && t.key) ? t.key : keyFrom(e.target.value) })} />
              <Switch id={`type-vpn-${i}`} checked={t.vpn} onChange={(v) => setType(i, { vpn: v })} label="VPN" />
              <Switch id={`type-on-${i}`} checked={t.active} onChange={(v) => setType(i, { active: v })} label="In use" />
              {!start.types.some((s) => s.key === t.key) && <Button variant="ghost" aria-label="Remove" onClick={() => setTypes(types.filter((_, j) => j !== i))}><X size={15} /></Button>}
            </li>
          ))}
        </ul>
        {errors.types && <p role="alert" className="px-4 pb-3 text-[13px] text-danger">{errors.types}</p>}
      </Card>
      <div className="space-y-4">
        <Card title="Sites">
          <div className="p-4"><Field label="Locations links start from, one per line" htmlFor="link-sites" error={errors.sites}>
            <textarea id="link-sites" className="min-h-28 w-full rounded-[5px] border border-line-strong bg-surface-2 px-2.5 py-2 text-[13px] text-ink focus:border-accent focus:bg-surface focus:outline-none" value={sites} onChange={(e) => setSites(e.target.value)} />
          </Field></div>
        </Card>
        <Card title="Contract reminders and currency">
          <div className="space-y-3 p-4">
            <Field label="Remind this many days before expiry" htmlFor="link-days" error={errors.reminderDays} hint="0 is the expiry date itself. Each reminder goes once per contract.">
              <Input id="link-days" value={days} onChange={(e) => setDays(e.target.value)} />
            </Field>
            <Field label="Approved rate: LKR per USD" htmlFor="link-rate" error={errors.usdRate} hint="Leave empty to show each currency's total on its own.">
              <Input id="link-rate" inputMode="decimal" value={rate} onChange={(e) => setRate(e.target.value)} />
            </Field>
          </div>
        </Card>
        <FormError error={save.error} />
        <div className="flex justify-end"><Button variant="primary" busy={save.isPending} onClick={submit}>Save</Button></div>
      </div>
    </div>
  )
}
