import { useMutation, useQuery } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Button, Card, Details, ErrorState, Field, FormError, Input, Notice, SkeletonRows, StateBadge, Switch, useConfirmSave } from '../../components/ui'
import { get, post, put } from '../../lib/api'
import { formatAgo } from '../../lib/format'
import { useInvalidate } from '../../lib/queries'
import { listChanges } from '../../lib/changes'

interface Intune {
  config: { enabled: boolean; intervalMinutes: number }
  status: { running: boolean; lastRun: string | null; lastOk: string | null; error: string; devices: number; matched: number; entraError: string; entraDevices: number; entraMatched: number }
  configured: boolean
}

/**
 * The optional Microsoft Intune connection, which also reads Entra ID. It
 * names the person who uses each discovered device and adds Intune's record of
 * it: evidence only, never the register's or device's owner.
 */
export function IntuneTab() {
  const q = useQuery({ queryKey: ['/integrations/intune'], queryFn: () => get<Intune>('/integrations/intune'), refetchInterval: (x) => (x.state.data?.status.running ? 3_000 : 30_000) })
  const refresh = useInvalidate('/integrations/intune', '/discovery')
  const confirmSave = useConfirmSave()
  const [form, setForm] = useState({ enabled: false, intervalMinutes: '60' })
  useEffect(() => { if (q.data) setForm({ enabled: q.data.config.enabled, intervalMinutes: String(q.data.config.intervalMinutes) }) }, [q.data])
  const save = useMutation({
    mutationFn: () => put('/integrations/intune', { enabled: form.enabled, intervalMinutes: Number(form.intervalMinutes) }),
    meta: { inlineError: true, success: 'Intune settings saved.' }, onSuccess: () => void refresh(),
  })
  const sync = useMutation({ mutationFn: () => post('/integrations/intune/sync'), meta: { success: 'Reading Intune now.' }, onSuccess: () => void refresh() })
  if (q.isPending) return <Card><SkeletonRows rows={4} /></Card>
  if (q.isError) return <Card><ErrorState error={q.error} onRetry={() => void q.refetch()} /></Card>
  const { status, config, configured } = q.data
  const interval = Number(form.intervalMinutes)
  const bad = !Number.isInteger(interval) || interval < 15 || interval > 1440
  const changes = listChanges({ enabled: config.enabled, intervalMinutes: String(config.intervalMinutes) }, form, { enabled: 'Read on a schedule', intervalMinutes: 'Read every (minutes)' })

  return (
    <div className="grid gap-4 xl:grid-cols-[1fr_380px]">
      <Card title="Microsoft Intune" subtitle="Optional and read-only. Shows who uses each discovered device, and its serial, model, compliance and encryption, from Intune through Microsoft Graph. Devices only in Entra ID get their registered owner.">
        <div className="space-y-4 p-4">
          {!configured && <Notice tone="warn">The Entra app isn't set up in api/.env (ENTRA_TENANT_ID, ENTRA_CLIENT_ID, ENTRA_CLIENT_SECRET), so Intune and Entra ID can't be read.</Notice>}
          <Switch id="in-on" checked={form.enabled} onChange={(v) => setForm((f) => ({ ...f, enabled: v }))} label="Read Intune on a schedule" hint="Off by default. “Read now” works while it is off, so you can try it first." />
          <Field label="Read every" htmlFor="in-int" error={bad ? 'From 15 to 1440 minutes.' : undefined}>
            <div className="flex items-center gap-2"><Input id="in-int" inputMode="numeric" className="w-24" value={form.intervalMinutes} onChange={(e) => setForm((f) => ({ ...f, intervalMinutes: e.target.value }))} /><span className="text-[13px] text-ink-2">minutes</span></div>
          </Field>
          <FormError error={save.error} />
          <div className="flex flex-wrap gap-2 border-t border-line pt-4">
            <Button variant="primary" disabled={bad || changes.length === 0} busy={save.isPending} onClick={() => void confirmSave('the Intune connection', () => save.mutate(), 'Saved settings take effect on the next read and are recorded in the audit log.', changes)}>Save</Button>
            <Button disabled={!configured || status.running} busy={sync.isPending || status.running} onClick={() => sync.mutate()}>{!status.running && <RefreshCw size={14} aria-hidden />}Read now</Button>
          </div>
        </div>
      </Card>
      <div className="space-y-4">
        <Card title="Last read">
          <div className="space-y-3 p-4">
            {status.running ? <StateBadge tone="accent">Reading now…</StateBadge>
              : status.error || status.entraError ? <>
                  {status.error && <Notice tone="danger">Intune: {status.error}.</Notice>}
                  {status.entraError && <Notice tone="danger">Entra ID: {status.entraError}.</Notice>}
                </>
              : status.lastOk ? <StateBadge tone="ok">Read {formatAgo(status.lastOk)}</StateBadge>
              : <p className="text-[13px] text-ink-3">Not read since the server started.</p>}
            <Details items={[['Intune devices', status.devices], ['Discovered devices found in Intune', status.matched], ['Entra ID devices', status.entraDevices], ['Discovered devices found in Entra ID', status.entraMatched], ['Last attempt', status.lastRun ? formatAgo(status.lastRun) : '—']]} />
          </div>
        </Card>
        <Card title="Setting it up">
          <ul className="list-disc space-y-1.5 py-3 pr-4 pl-8 text-[12.5px] text-ink-2">
            <li>In Entra ID, give DF-IPAM's app registration the Microsoft Graph <strong className="text-ink">application</strong> permission <span className="font-mono">DeviceManagementManagedDevices.Read.All</span>, then <strong className="text-ink">Grant admin consent</strong>. A tenant administrator does this once.</li>
            <li>For devices in Entra ID but not in Intune, also give it <span className="font-mono">Device.Read.All</span> and <span className="font-mono">User.Read.All</span>. Without them Intune is still read, and Entra ID shows an error here.</li>
            <li>It uses the same app and secret as sign-in. No new secret is stored.</li>
            <li>Each Intune device is matched by Wi-Fi or Ethernet MAC, or by computer name when only one Intune device has it. Discovery then shows its primary user, serial number, maker and model, OS, compliance, disk encryption and last check-in.</li>
            <li>Entra ID keeps no MAC addresses, so its devices match by computer name only, and only those with a registered owner give a user. PCs joined from the office domain (hybrid joined) often have no owner. Intune's user wins where both have one.</li>
            <li>Only GET requests are sent. Nothing in Intune or Entra is changed.</li>
            <li>A device that isn't in Intune, such as a personal laptop or phone, shows none of this. It is still found and flagged as unknown.</li>
          </ul>
        </Card>
      </div>
    </div>
  )
}
