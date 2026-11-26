import { useMutation } from '@tanstack/react-query'
import { Send } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Badge, Button, Card, Field, FormError, Input, Notice, Select, Switch, useConfirm, useConfirmSave } from '../../components/ui'
import { fieldIssues, post, put } from '../../lib/api'
import type { SystemSettings } from '../../lib/types'

/** Which email goes when, so an Admin can see that not everything is emailed. */
const delivery: { when: string; tone: 'danger' | 'warn' | 'accent' | 'neutral'; what: string }[] = [
  { when: 'At once', tone: 'danger', what: 'Critical alerts, and a decision on someone\'s own IP request.' },
  { when: 'Digest', tone: 'warn', what: 'High alerts and new IP requests, gathered into one email per interval.' },
  { when: 'Daily summary', tone: 'accent', what: 'Open alerts, devices and requests waiting, networks 90% full, failing FortiGate or Intune reads and scans, contracts ending within 30 days, reservations expiring. Only what needs someone; nothing on a quiet day.' },
  { when: 'App only', tone: 'neutral', what: 'Medium and Low alerts, servers back up, discovery findings and everything else.' },
]

const hours = Array.from({ length: 24 }, (_, h) => h)

/** Email settings: the switch, the team mailbox, when digests and the summary go, and a test email. */
export function EmailSettings({ settings: s, onSaved }: { settings: SystemSettings; onSaved: () => void }) {
  const confirm = useConfirm()
  const confirmSave = useConfirmSave()
  const initial = { team: s.teamAddress, digest: String(s.values.digestMinutes ?? 30), hour: String(s.values.summaryHour ?? 8) }
  const [v, setV] = useState(initial)
  const saved = JSON.stringify(initial)
  useEffect(() => setV(JSON.parse(saved) as typeof initial), [saved])
  const toggle = useMutation({
    mutationFn: (on: boolean) => put('/settings', { emailEnabled: on }),
    meta: { success: (on: boolean) => (on ? 'Email switched on.' : 'Email switched off.') }, onSuccess: onSaved,
  })
  const save = useMutation({
    mutationFn: () => put('/settings', { teamAddress: v.team.trim(), values: { digestMinutes: Number(v.digest), summaryHour: Number(v.hour) } }),
    meta: { inlineError: true, success: 'Email settings saved.' }, onSuccess: onSaved,
  })
  const test = useMutation({
    mutationFn: () => post<{ sentTo: string[] }>('/notifications/test-email'),
    meta: { inlineError: true, success: (_: unknown, r: { sentTo: string[] }) => `Test email sent to ${r.sentTo.join(' and ')}.` },
  })
  const issues = fieldIssues(save.error)
  const digest = Number(v.digest)
  const digestError = !Number.isInteger(digest) || digest < 15 || digest > 240 ? '15 to 240.' : issues['values.digestMinutes']
  const changes = [
    v.team.trim() !== s.teamAddress && `Team mailbox: ${s.teamAddress || 'none'} → ${v.team.trim() || 'none'}`,
    v.digest !== initial.digest && `Digest every: ${initial.digest} → ${v.digest} minutes`,
    v.hour !== initial.hour && `Daily summary at: ${initial.hour.padStart(2, '0')}:00 → ${v.hour.padStart(2, '0')}:00`,
  ].filter(Boolean) as string[]
  return (
    <Card title="Email" subtitle="Everything appears under the bell. Email is kept for what needs someone, and gathered so it doesn't pile up.">
      <div className="space-y-5 p-4">
        <Switch id="set-email" checked={s.emailEnabled} disabled={toggle.isPending} label="Send email" hint="Off by default. Each person can still switch kinds off in Alerts › Email preferences."
          onChange={async (on) => {
            if (await confirm({ title: on ? 'Switch email on?' : 'Switch email off?', tone: 'primary', confirmLabel: on ? 'Switch on' : 'Switch off',
              body: on ? 'People start getting the emails below, for the kinds they keep on.' : 'Nobody gets email. Notices still appear under the bell.' })) toggle.mutate(on)
          }} />
        {!s.smtpConfigured && <Notice tone="warn">No mail server is set in api/.env (SMTP_HOST), so no email can go out until one is configured.</Notice>}

        <div className="overflow-hidden rounded-md border border-line">
          {delivery.map((d, i) => (
            <div key={d.when} className={`flex flex-col gap-1 px-3 py-2.5 sm:flex-row sm:items-start sm:gap-3 ${i ? 'border-t border-line' : ''}`}>
              <div className="w-32 shrink-0"><Badge tone={d.tone}>{d.when}</Badge></div>
              <p className="text-[13px] text-ink-2">{d.what}</p>
            </div>
          ))}
        </div>

        <div className="grid gap-4 md:grid-cols-3">
          <Field label="Team mailbox" htmlFor="set-team" error={issues.teamAddress}
            hint="Optional. Alert and request email goes here once instead of to each person, for example a distribution list. Personal email still goes to the person.">
            <Input id="set-team" type="email" placeholder="netops@dialogfinance.lk" value={v.team} onChange={(e) => setV((x) => ({ ...x, team: e.target.value }))} />
          </Field>
          <Field label="Send a digest every" htmlFor="set-digest" error={digestError} hint="High alerts and new requests wait at most this long. 30 minutes by default.">
            <div className="flex items-center gap-2"><Input id="set-digest" inputMode="numeric" className="w-24" value={v.digest} onChange={(e) => setV((x) => ({ ...x, digest: e.target.value }))} /><span className="text-[13px] text-ink-2">minutes</span></div>
          </Field>
          <Field label="Send the daily summary at" htmlFor="set-hour" hint="Sri Lanka time, once a day.">
            <Select id="set-hour" className="w-32" value={v.hour} onChange={(e) => setV((x) => ({ ...x, hour: e.target.value }))}>
              {hours.map((h) => <option key={h} value={h}>{String(h).padStart(2, '0')}:00</option>)}
            </Select>
          </Field>
        </div>

        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line pt-4">
          <div className="flex items-center gap-3">
            <Button disabled={!s.smtpConfigured} busy={test.isPending} onClick={() => test.mutate()}><Send size={14} aria-hidden />Send a test email</Button>
            <FormError error={test.error} />
          </div>
          <div className="flex items-center gap-3">
            <FormError error={Object.keys(issues).length ? null : save.error} />
            <Button disabled={!changes.length} onClick={() => setV(initial)}>Discard</Button>
            <Button variant="primary" disabled={!changes.length || Boolean(digestError)} busy={save.isPending}
              onClick={() => void confirmSave('email settings', () => save.mutate(), undefined, changes)}>Save email settings</Button>
          </div>
        </div>
      </div>
    </Card>
  )
}
