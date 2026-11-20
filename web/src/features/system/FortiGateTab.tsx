import { useMutation, useQuery } from '@tanstack/react-query'
import { CheckCircle2, PlugZap, RefreshCw, Trash2, XCircle } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Button, Card, Details, ErrorState, Field, FormError, Input, Notice, SkeletonRows, StateBadge, Switch, useConfirm, useConfirmSave } from '../../components/ui'
import { del, get, post, put } from '../../lib/api'
import { formatAgo } from '../../lib/format'
import { useInvalidate } from '../../lib/queries'
import { listChanges } from '../../lib/changes'

interface FortiGate {
  config: { active: boolean; enabled: boolean; url: string; vdom: string; verifyTls: boolean; intervalMinutes: number; hasToken: boolean }
  status: { running: boolean; lastRun: string | null; lastOk: string | null; error: string; counts: { arp: number; dhcp: number; vpn: number }; firewall: string }
  encryptionReady: boolean
}

/**
 * The optional FortiGate connection. DF-IPAM only reads from it: ARP entries,
 * DHCP leases and VPN sessions become evidence, never register changes.
 */
export function FortiGateTab() {
  const q = useQuery({ queryKey: ['/integrations/fortigate'], queryFn: () => get<FortiGate>('/integrations/fortigate'), refetchInterval: (x) => (x.state.data?.status.running ? 3_000 : 30_000) })
  const refresh = useInvalidate('/integrations/fortigate', '/discovery')
  const confirmSave = useConfirmSave()
  const confirm = useConfirm()
  const [form, setForm] = useState({ active: true, enabled: false, url: '', vdom: '', verifyTls: true, intervalMinutes: '10', token: '' })
  useEffect(() => { if (q.data) setForm({ ...q.data.config, intervalMinutes: String(q.data.config.intervalMinutes), token: '' }) }, [q.data])
  const save = useMutation({
    mutationFn: () => put('/integrations/fortigate', { ...form, intervalMinutes: Number(form.intervalMinutes) }),
    meta: { inlineError: true, success: 'FortiGate settings saved.' }, onSuccess: () => { void refresh(); setForm((f) => ({ ...f, token: '' })) },
  })
  const test = useMutation({ mutationFn: () => post<{ ok: boolean; firewall?: string; error?: string }>('/integrations/fortigate/test'), meta: { inlineError: true } })
  const removeToken = useMutation({
    mutationFn: () => del('/integrations/fortigate/token'),
    meta: { success: 'FortiGate API token deleted.' }, onSuccess: () => { void refresh(); test.reset() },
  })
  const sync = useMutation({ mutationFn: () => post('/integrations/fortigate/sync'), meta: { success: 'Reading the FortiGate now.' }, onSuccess: () => void refresh() })
  if (q.isPending) return <Card><SkeletonRows rows={6} /></Card>
  if (q.isError) return <Card><ErrorState error={q.error} onRetry={() => void q.refetch()} /></Card>
  const { status, config, encryptionReady } = q.data
  const interval = Number(form.intervalMinutes)
  const urlIssue = form.url && !form.url.startsWith('https://') ? 'Use an https:// address.' : form.enabled && !form.url ? 'Enter the address to switch the connection on.' : undefined
  const bad = Boolean(urlIssue) || !Number.isInteger(interval) || interval < 5 || interval > 1440
  const saved = { active: config.active, enabled: config.enabled, url: config.url, vdom: config.vdom, verifyTls: config.verifyTls, intervalMinutes: String(config.intervalMinutes), token: '' }
  const changes = [
    ...listChanges(saved, form, { active: 'Connection', enabled: 'Read on a schedule', url: 'Address', vdom: 'VDOM', intervalMinutes: 'Read every (minutes)', verifyTls: 'Verify certificate' }),
    ...(form.token ? [config.hasToken ? 'API token: replaced' : 'API token: added'] : []),
  ]
  const set = <K extends keyof typeof form>(k: K, v: (typeof form)[K]) => setForm((f) => ({ ...f, [k]: v }))

  return (
    <div className="grid gap-4 xl:grid-cols-[1fr_380px]">
      <Card title="FortiGate connection" subtitle="Optional and read-only. Uses the FortiOS REST API with a token from a read-only admin profile.">
        <div className="space-y-4 p-4">
          {!encryptionReady && <Notice tone="warn">SECRET_ENCRYPTION_KEY is not set in api/.env, so an API token can't be stored. Set it and restart the backend.</Notice>}
          <Switch id="fg-active" checked={form.active} onChange={(v) => set('active', v)} label="Connection on"
            hint={form.active ? 'DF-IPAM may read the FortiGate: scheduled reads, Read now, Firewall > Live and Firewall > Import.' : 'Off: DF-IPAM doesn\'t contact the FortiGate at all. Scheduled reads, Read now, Live and Import stop until it is switched back on. The settings and token are kept.'} />
          {config.active === false && <Notice tone="warn">The connection is switched off. Nothing reads the FortiGate until it is switched on and saved.</Notice>}
          <Switch id="fg-on" checked={form.enabled} onChange={(v) => set('enabled', v)} label="Read the FortiGate on a schedule" hint="Records its address table, DHCP leases and VPN sessions as Discovery evidence, which raises alerts and feeds reports. Firewall > Live and Review read it when you look, and don't need this. Off by default; “Read now” works while it is off." />
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Address" htmlFor="fg-url" error={urlIssue} hint="Management address, with the port if not 443."><Input id="fg-url" className="font-mono" placeholder="https://10.0.0.1" value={form.url} onChange={(e) => set('url', e.target.value)} /></Field>
            <Field label="VDOM (optional)" htmlFor="fg-vdom"><Input id="fg-vdom" placeholder="root" value={form.vdom} onChange={(e) => set('vdom', e.target.value)} /></Field>
            <Field label="API token" htmlFor="fg-token" hint={config.hasToken ? 'A token is stored, encrypted. Leave empty to keep it.' : 'Stored encrypted. It is never shown again.'}>
              <div className="flex items-center gap-2">
                <Input id="fg-token" type="password" autoComplete="off" placeholder={config.hasToken ? '•••••••• stored' : 'Paste the REST API token'} value={form.token} onChange={(e) => set('token', e.target.value)} />
                {config.hasToken && (
                  <Button variant="danger" busy={removeToken.isPending} aria-label="Delete the stored API token" onClick={async () => {
                    if (await confirm({ title: 'Delete the FortiGate API token?', body: 'The stored token is erased and the scheduled read is switched off. DF-IPAM can\'t read the firewall until a new token is saved. The address and other settings are kept. Recorded in the audit log. Also revoke the token on the FortiGate if it is no longer needed.', confirmLabel: 'Delete token' })) removeToken.mutate()
                  }}><Trash2 size={14} aria-hidden />Delete</Button>
                )}
              </div>
            </Field>
            <Field label="Read every" htmlFor="fg-int" error={bad && !urlIssue ? 'From 5 to 1440 minutes.' : undefined}>
              <div className="flex items-center gap-2"><Input id="fg-int" inputMode="numeric" className="w-24" value={form.intervalMinutes} onChange={(e) => set('intervalMinutes', e.target.value)} /><span className="text-[13px] text-ink-2">minutes</span></div>
            </Field>
          </div>
          <Switch id="fg-tls" checked={form.verifyTls} onChange={(v) => set('verifyTls', v)} label="Verify the firewall's certificate"
            hint={form.verifyTls ? 'Recommended.' : 'Off: only for a firewall with a self-signed certificate on a trusted management network.'} />
          <FormError error={save.error} />
          <div className="flex flex-wrap gap-2 border-t border-line pt-4">
            <Button variant="primary" disabled={bad || changes.length === 0} busy={save.isPending} onClick={() => void confirmSave('the FortiGate connection', () => save.mutate(), 'Saved settings take effect on the next read and are recorded in the audit log. The token is never logged.', changes)}>Save</Button>
            <Button disabled={!config.hasToken || !config.active} busy={test.isPending} onClick={() => test.mutate()}><PlugZap size={14} aria-hidden />Test connection</Button>
            <Button disabled={!config.hasToken || !config.active || status.running} busy={sync.isPending || status.running} onClick={() => sync.mutate()}>{!status.running && <RefreshCw size={14} aria-hidden />}Read now</Button>
          </div>
          {test.data && (test.data.ok
            ? <Notice tone="ok"><CheckCircle2 size={14} className="mr-1 inline" aria-hidden />Connected to {test.data.firewall || 'the FortiGate'}.</Notice>
            : <Notice tone="danger"><XCircle size={14} className="mr-1 inline" aria-hidden />{test.data.error}</Notice>)}
          {test.error && <FormError error={test.error} />}
        </div>
      </Card>
      <div className="space-y-4">
        <Card title="Last read">
          <div className="space-y-3 p-4">
            {status.running ? <StateBadge tone="accent">Reading now…</StateBadge>
              : status.error ? <Notice tone="danger">{status.error}</Notice>
              : status.lastOk ? <StateBadge tone="ok">Read {formatAgo(status.lastOk)}{status.firewall ? ` from ${status.firewall}` : ''}</StateBadge>
              : <p className="text-[13px] text-ink-3">Not read since the server started.</p>}
            <Details items={[['ARP entries', status.counts.arp], ['DHCP leases', status.counts.dhcp], ['VPN clients', status.counts.vpn], ['Last attempt', status.lastRun ? formatAgo(status.lastRun) : '—']]} />
          </div>
        </Card>
        <Card title="What DF-IPAM reads">
          <ul className="list-disc space-y-1.5 py-3 pr-4 pl-8 text-[12.5px] text-ink-2">
            <li><strong className="text-ink">ARP table:</strong> IP and MAC of every device the firewall has talked to, so MACs appear even across routed floors.</li>
            <li><strong className="text-ink">DHCP leases:</strong> hostname, MAC and the DHCP vendor class (for example “MSFT 5.0”), which hints at the OS.</li>
            <li><strong className="text-ink">VPN sessions:</strong> SSL and IPsec users with their tunnel IP and where they connected from. Add the VPN client pool as a VPN segment in Network setup to group them.</li>
            <li>Only GET requests are sent. Nothing on the firewall is changed, and no traffic is inspected.</li>
            <li>Create a REST API admin with a read-only profile, and limit its trusted hosts to the DF-IPAM server.</li>
          </ul>
        </Card>
      </div>
    </div>
  )
}
