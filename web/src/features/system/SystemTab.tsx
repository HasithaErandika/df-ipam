import { useMutation, useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { Button, Card, ErrorState, Field, FormError, Input, Notice, SkeletonRows, Switch, useConfirm, useConfirmSave } from '../../components/ui'
import { fieldIssues, get, put } from '../../lib/api'
import { useInvalidate } from '../../lib/queries'
import type { SystemSettings } from '../../lib/types'

interface NumberField { key: string; label: string; unit: string; min: number; max: number; hint: string }

const scanning: NumberField[] = [
  { key: 'scanIntervalMinutes', label: 'Discover devices every', unit: 'minutes', min: 5, max: 1440, hint: 'Ping sweep of every floor and server segment. 10 minutes is a gentle default; 5 is the least, so scans never run back to back.' },
  { key: 'serverCheckSeconds', label: 'Check monitored servers every', unit: 'seconds', min: 60, max: 86400, hint: '86400 is once a day. Shorter intervals find outages sooner.' },
  { key: 'downAfterFailures', label: 'Mark a server Down after', unit: 'failed checks', min: 1, max: 20, hint: 'In a row. With daily checks, 1 reports an outage on the first failed check.' },
]
const other: NumberField[] = [
  { key: 'reminderDays', label: 'Remind reservation owners', unit: 'days before expiry', min: 1, max: 60, hint: 'One reminder per reservation.' },
  { key: 'sessionTimeoutMinutes', label: 'Sign people out after', unit: 'minutes without activity', min: 5, max: 480, hint: 'A warning shows one minute before. Activity in any tab counts.' },
]

function describe(seconds: number) {
  if (seconds >= 86400 && seconds % 86400 === 0) return seconds === 86400 ? 'once a day' : `every ${seconds / 86400} days`
  if (seconds >= 3600 && seconds % 3600 === 0) return `every ${seconds / 3600} h`
  if (seconds >= 60 && seconds % 60 === 0) return `every ${seconds / 60} min`
  return `every ${seconds} s`
}

export function SystemTab() {
  const q = useQuery({ queryKey: ['/settings'], queryFn: () => get<SystemSettings>('/settings') })
  const [values, setValues] = useState<Record<string, string>>({})
  const refresh = useInvalidate('/settings', '/notification-preferences', '/discovery', 'me')
  const confirmSave = useConfirmSave()
  const confirm = useConfirm()
  useEffect(() => { if (q.data) setValues(Object.fromEntries(Object.entries(q.data.values).map(([k, v]) => [k, String(v)]))) }, [q.data])
  const save = useMutation({
    mutationFn: (body: object) => put('/settings', body),
    meta: { inlineError: true, success: 'Settings saved.' }, onSuccess: () => void refresh(),
  })
  const email = useMutation({
    mutationFn: (on: boolean) => put('/settings', { emailEnabled: on }),
    meta: { success: (on: boolean) => (on ? 'Email notifications switched on.' : 'Email notifications switched off.') }, onSuccess: () => void refresh(),
  })
  if (q.isPending) return <Card><SkeletonRows rows={6} /></Card>
  if (q.isError) return <Card><ErrorState error={q.error} onRetry={() => void q.refetch()} /></Card>
  const s = q.data
  const issues = fieldIssues(save.error)
  const invalid = (f: NumberField) => {
    const n = Number(values[f.key])
    return !Number.isInteger(n) || n < f.min || n > f.max ? `${f.min} to ${f.max}.` : issues[`values.${f.key}`]
  }
  const all = [...scanning, ...other]
  const dirty = all.some((f) => String(s.values[f.key]) !== values[f.key])
  const inRange = (f: NumberField) => { const n = Number(values[f.key]); return Number.isInteger(n) && n >= f.min && n <= f.max }
  const ok = all.every(inRange)
  const changes = all.filter((f) => String(s.values[f.key]) !== values[f.key]).map((f) => `${f.label}: ${s.values[f.key]} → ${values[f.key]} ${f.unit}`)
  const row = (f: NumberField) => (
    <Field key={f.key} label={f.label} htmlFor={`set-${f.key}`} error={invalid(f)} hint={f.key === 'serverCheckSeconds' && !invalid(f) ? `${f.hint} Now: ${describe(Number(values[f.key]))}.` : f.hint}>
      <div className="flex items-center gap-2"><Input id={`set-${f.key}`} inputMode="numeric" className="w-28" value={values[f.key] ?? ''} onChange={(e) => setValues((v) => ({ ...v, [f.key]: e.target.value }))} /><span className="text-[13px] text-ink-2">{f.unit}</span></div>
    </Field>
  )
  return (
    <div className="space-y-4">
      <Card title="Email notifications" subtitle="Notifications always appear under the bell. This switch controls email for everyone.">
        <div className="space-y-3 p-4">
          <Switch id="set-email" checked={s.emailEnabled} disabled={email.isPending} onChange={async (on) => {
            if (await confirm({ title: on ? 'Switch email notifications on?' : 'Switch email notifications off?', tone: 'primary', confirmLabel: on ? 'Switch on' : 'Switch off',
              body: on ? 'Everyone starts getting emails for the notification types they keep on.' : 'Nobody gets email; notifications still appear under the bell.' })) email.mutate(on)
          }}
            label="Send email notifications" hint="High and Critical alerts, request decisions, servers back up and expiring reservations. Each person can switch types off." />
          {!s.smtpConfigured && <Notice tone="warn">No mail server is set in api/.env (SMTP_HOST). Email stays off in practice until one is configured.</Notice>}
          {s.emailEnabled && s.smtpConfigured && <Notice tone="ok">Email is on. At most 20 emails an hour go to one person; a burst of alerts arrives as one summary.</Notice>}
        </div>
      </Card>
      <Card title="Discovery and checks" subtitle={`Method: ${s.scanMethod === 'ping' ? 'ping sweep with reverse DNS and NetBIOS (option C)' : s.scanMethod}. Read-only: nothing on the network is changed.`}>
        <div className="grid gap-4 p-4 md:grid-cols-3">{scanning.map(row)}</div>
      </Card>
      <Card title="Reservations and sessions">
        <div className="grid gap-4 p-4 md:grid-cols-2">{other.map(row)}</div>
      </Card>
      <div className="flex items-center justify-end gap-3">
        <FormError error={Object.keys(issues).length ? null : save.error} />
        <Button disabled={!dirty} onClick={() => setValues(Object.fromEntries(Object.entries(s.values).map(([k, v]) => [k, String(v)])))}>Discard</Button>
        <Button variant="primary" disabled={!dirty || !ok} busy={save.isPending}
          onClick={() => void confirmSave('system settings', () => save.mutate({ values: Object.fromEntries(all.map((f) => [f.key, Number(values[f.key])])) }), undefined, changes)}>Save settings</Button>
      </div>
    </div>
  )
}
