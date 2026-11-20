import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { Badge, Button, Field, FormError, FormSection, Input, Modal, Select, Switch, Textarea, useConfirmSave } from '../../components/ui'
import { post, put } from '../../lib/api'
import { applyServerErrors } from '../../lib/forms'
import { formatCount } from '../../lib/format'
import { inventoryKeys, useEnvironments, useInvalidate } from '../../lib/queries'
import { listChanges } from '../../lib/changes'
import type { Segment } from '../../lib/types'
import { AddressLayout, type NetworkInfo } from '../discovery/SegmentFacts'
import { cidrIssue, inNetwork, ipToInt, ipv4NetworkInfo, isHostAddress, isIPv4, maxScannable, optionalInt, optionalIPv4, requiredText, text, toInt } from '../../lib/validation'
import { notSwept } from './segmentList'

const schema = z.object({
  name: requiredText('Name', 80),
  kind: z.enum(['dhcp', 'server', 'cloud', 'vpn']),
  floor: optionalInt('Floor', -5, 200),
  vlan: optionalInt('VLAN', 1, 4094),
  network: z.string().trim().superRefine((v, ctx) => { const issue = cidrIssue(v); if (issue) ctx.addIssue({ code: 'custom', message: issue }) }),
  gateway: optionalIPv4,
  dhcpStart: optionalIPv4,
  dhcpEnd: optionalIPv4,
  defaultEnvironmentId: z.string(),
  description: text('Description', 500),
  alertPrivateMacs: z.boolean(),
  checkPorts: z.boolean(),
}).superRefine((v, ctx) => {
  const ok = !cidrIssue(v.network)
  for (const k of ['gateway', 'dhcpStart', 'dhcpEnd'] as const) {
    if (ok && v[k] && !inNetwork(v[k], v.network)) ctx.addIssue({ code: 'custom', path: [k], message: `Must be inside ${v.network}.` })
  }
  if (v.kind === 'dhcp' && Boolean(v.dhcpStart) !== Boolean(v.dhcpEnd)) ctx.addIssue({ code: 'custom', path: ['dhcpEnd'], message: 'Enter both the first and last pool address.' })
  if (v.dhcpStart && v.dhcpEnd && ipToInt(v.dhcpEnd) < ipToInt(v.dhcpStart)) ctx.addIssue({ code: 'custom', path: ['dhcpEnd'], message: 'The last pool address must not be before the first.' })
  if (!ok) return
  if (v.network.endsWith('/32') && (v.gateway || v.dhcpStart)) ctx.addIssue({ code: 'custom', path: ['network'], message: 'A single-address segment (/32) has no gateway or DHCP pool.' })
  const host = (k: 'gateway' | 'dhcpStart' | 'dhcpEnd', message: string) => {
    if (v[k] && inNetwork(v[k], v.network) && !isHostAddress(v[k], v.network)) ctx.addIssue({ code: 'custom', path: [k], message })
  }
  host('gateway', 'Use a host address, not the network or broadcast address.')
  host('dhcpStart', "The pool can't start at the network address.")
  host('dhcpEnd', "The pool can't include the broadcast address.")
})
type Values = z.infer<typeof schema>
const fields = Object.keys(schema.shape)

const initial = (s?: Segment): Values => ({
  name: s?.name ?? '', kind: s?.kind ?? 'dhcp', floor: s?.floor?.toString() ?? '', vlan: s?.vlan?.toString() ?? '', network: s?.network ?? '',
  gateway: s?.gateway ?? '', dhcpStart: s?.dhcpStart ?? '', dhcpEnd: s?.dhcpEnd ?? '', defaultEnvironmentId: s?.defaultEnvironmentId ?? '', description: s?.description ?? '',
  alertPrivateMacs: s?.alertPrivateMacs ?? false,
  checkPorts: s?.checkPorts ?? false,
})

const labels: Partial<Record<keyof Values, string>> = {
  name: 'Name', kind: 'Type', floor: 'Floor', vlan: 'VLAN', network: 'Network', gateway: 'Gateway', dhcpStart: 'DHCP pool start',
  dhcpEnd: 'DHCP pool end', defaultEnvironmentId: 'Default environment', description: 'Description',
  alertPrivateMacs: 'Alert on private-MAC devices', checkPorts: 'Check common ports',
}

const kindHelp = {
  dhcp: 'A user network served by DHCP, such as a floor or Wi-Fi. Addresses outside the pool count as static.',
  server: 'Static server addresses. Entries take the default environment.',
  cloud: 'A network hosted in a cloud. Static, like servers, and never ping-swept from the building.',
  vpn: 'The address pool the FortiGate gives VPN users (SSL or IPsec). Never swept: the FortiGate connection shows who is connected.',
}

export function SegmentForm({ open, segment, onClose }: { open: boolean; segment?: Segment; onClose: () => void }) {
  const environments = useEnvironments()
  const refresh = useInvalidate(...inventoryKeys)
  const confirmSave = useConfirmSave()
  const [error, setFormError] = useState<unknown>(null)
  const { register, handleSubmit, reset, watch, setValue, setError, formState: { errors, isDirty } } = useForm<Values>({ resolver: zodResolver(schema), defaultValues: initial(segment) })
  useEffect(() => { if (open) { reset(initial(segment)); setFormError(null) } }, [open, segment, reset])
  const kind = watch('kind')
  const networkValue = watch('network')
  const subnet = ipv4NetworkInfo(networkValue)
  const preview: NetworkInfo | null = subnet ? {
    ...subnet, totalAddresses: String(subnet.totalAddresses), usableAddresses: String(subnet.usableAddresses),
  } : null
  const poolStart = watch('dhcpStart')
  const poolEnd = watch('dhcpEnd')
  const gateway = watch('gateway')
  const validPool = Boolean(preview && kind === 'dhcp' && isIPv4(poolStart) && isIPv4(poolEnd)
    && inNetwork(poolStart, networkValue) && inNetwork(poolEnd, networkValue) && ipToInt(poolEnd) >= ipToInt(poolStart))
  const validGateway = Boolean(preview && subnet && isIPv4(gateway) && inNetwork(gateway, networkValue)
    && ipToInt(gateway) >= ipToInt(subnet.firstUsable) && ipToInt(gateway) <= ipToInt(subnet.lastUsable))
  const poolUsableCount = validPool && subnet
    ? Math.max(0, Math.min(ipToInt(poolEnd), ipToInt(subnet.lastUsable)) - Math.max(ipToInt(poolStart), ipToInt(subnet.firstUsable)) + 1)
    : 0
  const gatewayInPool = validGateway && validPool && ipToInt(gateway) >= ipToInt(poolStart) && ipToInt(gateway) <= ipToInt(poolEnd)
  const estimatedAvailable = subnet ? Math.max(0, subnet.usableAddresses - poolUsableCount - (validGateway && !gatewayInPool ? 1 : 0)) : 0
  const save = useMutation({
    mutationFn: (v: Values) => {
      const body = { ...v, floor: toInt(v.floor), vlan: toInt(v.vlan), dhcpStart: kind === 'dhcp' ? v.dhcpStart : '', dhcpEnd: kind === 'dhcp' ? v.dhcpEnd : '', defaultEnvironmentId: kind === 'dhcp' ? '' : v.defaultEnvironmentId }
      return segment ? put(`/segments/${segment.id}`, body) : post('/segments', body)
    },
    meta: { inlineError: true, success: (v: Values) => (segment ? `Saved ${v.name}.` : `${v.name} added. It is scanned on the next run.`) },
    onSuccess: () => { void refresh(); onClose() },
    onError: (e) => { if (!applyServerErrors(e, setError, fields)) setFormError(e) },
  })
  const envName = (id: unknown) => environments.data?.find((e) => e.id === id)?.name ?? 'none'
  const submit = handleSubmit((v) => (segment
    ? void confirmSave(segment.name, () => save.mutate(v), undefined, listChanges(initial(segment), v, labels, { defaultEnvironmentId: envName }))
    : save.mutate(v)))
  return (
    <Modal open={open} onClose={onClose} title={segment ? `Edit ${segment.name}` : 'Add a segment'} size="lg"
      footer={<><Button onClick={onClose}>Cancel</Button><Button variant="primary" busy={save.isPending} disabled={Boolean(segment) && !isDirty} title={segment && !isDirty ? 'Nothing has changed yet' : undefined} onClick={() => void submit()}>{segment ? 'Save changes' : 'Add segment'}</Button></>}>
      <form onSubmit={(e) => { e.preventDefault(); void submit() }} noValidate className="space-y-5">
        <FormSection title="Segment">
          <div className="grid gap-3 sm:grid-cols-2">
            <Field label="Name" htmlFor="sg-name" error={errors.name?.message}><Input id="sg-name" placeholder="Floor 4" {...register('name')} /></Field>
            <Field label="Type" htmlFor="sg-kind" hint={kindHelp[kind]}>
              <Select id="sg-kind" {...register('kind')}><option value="dhcp">DHCP network (floor, Wi-Fi)</option><option value="server">Server</option><option value="cloud">Cloud</option><option value="vpn">VPN client pool</option></Select>
            </Field>
            <Field label="Network" htmlFor="sg-net" error={errors.network?.message} hint="IPv4, /16 to /32. Use /30, /31 or /32 for a WAN hand-off or a single address. Networks can't overlap.">
              <Input id="sg-net" className="font-mono" placeholder="192.168.4.0/24" {...register('network')} />
            </Field>
            <Field label="Gateway (optional)" htmlFor="sg-gw" error={errors.gateway?.message}
              hint={kind === 'dhcp' && gatewayInPool ? 'Inside the DHCP pool. Make sure the DHCP server excludes this address from its leases.' : undefined}><Input id="sg-gw" className="font-mono" placeholder="192.168.4.1" {...register('gateway')} /></Field>
            {!notSwept(kind) && <Field label="Floor (optional)" htmlFor="sg-floor" error={errors.floor?.message}><Input id="sg-floor" inputMode="numeric" {...register('floor')} /></Field>}
            <Field label="VLAN (optional)" htmlFor="sg-vlan" error={errors.vlan?.message}><Input id="sg-vlan" inputMode="numeric" {...register('vlan')} /></Field>
          </div>
          {preview && (
            <div className="mt-4 rounded-md border border-line bg-surface p-3.5">
              <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                <div><h3 className="text-[13px] font-semibold text-ink">Address space preview</h3><p className="text-[11.5px] text-ink-3">Updates as you edit the network, gateway and DHCP pool.</p></div>
                <Badge tone="ok">Live</Badge>
              </div>
              <p className="mb-3 text-[12px] text-ink-2">Subnet mask <span className="font-mono font-medium">{preview.mask}</span><span className="mx-2 text-ink-3">·</span>Broadcast <span className="font-mono font-medium">{preview.broadcast}</span></p>
              <AddressLayout info={preview} poolStart={validPool ? poolStart : null} poolEnd={validPool ? poolEnd : null} gateway={validGateway ? gateway : null} />
              <div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
                <div className="rounded-md bg-surface-2 px-2.5 py-2"><div className="text-[11px] text-ink-3">Total addresses</div><div className="text-[13px] font-semibold text-ink">{formatCount(Number(preview.totalAddresses))}</div></div>
                <div className="rounded-md bg-surface-2 px-2.5 py-2"><div className="text-[11px] text-ink-3">Usable addresses</div><div className="text-[13px] font-semibold text-ink">{formatCount(Number(preview.usableAddresses))}</div><div className="truncate font-mono text-[10.5px] text-ink-3" title={`${preview.firstUsable} – ${preview.lastUsable}`}>{preview.firstUsable} – {preview.lastUsable}</div></div>
                <div className="rounded-md bg-surface-2 px-2.5 py-2"><div className="text-[11px] text-ink-3">Potential available</div><div className="text-[13px] font-semibold text-ok">{formatCount(estimatedAvailable)}</div></div>
                <div className="rounded-md bg-surface-2 px-2.5 py-2"><div className="text-[11px] text-ink-3">DHCP pool</div><div className="text-[13px] font-semibold text-ink">{validPool ? formatCount(ipToInt(poolEnd) - ipToInt(poolStart) + 1) : kind === 'dhcp' ? 'Not set' : 'None'}</div></div>
              </div>
              {!notSwept(kind) && subnet && subnet.usableAddresses > maxScannable && (
                <p role="alert" className="mt-3 rounded-md bg-warn-soft px-3 py-2 text-[12.5px] text-warn">
                  Too large to scan: {formatCount(subnet.usableAddresses)} usable addresses, and discovery sweeps at most {formatCount(maxScannable)} (a /20). Split it into smaller segments, or it is never scanned.
                </p>
              )}
              <p className="mt-2 text-[11.5px] text-ink-3">Potential available subtracts the entered gateway and DHCP pool. After saving, actual Available candidates also account for registered and reserved addresses and recent discovery.</p>
            </div>
          )}
        </FormSection>
        {kind === 'dhcp' ? (
          <FormSection title="DHCP pool">
            <div className="grid gap-3 sm:grid-cols-2">
              <Field label="First address" htmlFor="sg-ps" error={errors.dhcpStart?.message}><Input id="sg-ps" className="font-mono" placeholder="192.168.4.100" {...register('dhcpStart')} /></Field>
              <Field label="Last address" htmlFor="sg-pe" error={errors.dhcpEnd?.message}><Input id="sg-pe" className="font-mono" placeholder="192.168.4.250" {...register('dhcpEnd')} /></Field>
            </div>
            <p className="text-[12.5px] text-ink-3">Static detection depends on this range: anything in use outside it is treated as a hand-set IP. Without a pool, nothing here counts as static and no unregistered-static alerts are raised.</p>
          </FormSection>
        ) : kind === 'vpn' ? null : (
          <Field label="Default environment" htmlFor="sg-env" hint="New entries in this segment start with it. Each entry can still differ.">
            <Select id="sg-env" {...register('defaultEnvironmentId')}><option value="">None</option>{environments.data?.map((e) => <option key={e.id} value={e.id}>{e.name}</option>)}</Select>
          </Field>
        )}
        <FormSection title="Alerts">
          <Switch id="sg-private" checked={watch('alertPrivateMacs')} onChange={(v) => setValue('alertPrivateMacs', v, { shouldDirty: true })}
            label="Alert on devices with a private MAC"
            hint="Phones and laptops use random MACs on Wi-Fi, so these are not unknown-device alerts. Turn this on where every device matters: one alert per unregistered address, Medium on floors and High on server and cloud networks." />
        </FormSection>
        {!notSwept(watch('kind')) && (
          <FormSection title="Discovery">
            <Switch id="sg-ports" checked={watch('checkPorts')} onChange={(v) => setValue('checkPorts', v, { shouldDirty: true })}
              label="Check common ports on each device"
              hint="Tries about 25 common TCP ports (SSH, web, RDP, file sharing, databases, printers) on each device that answers, at most every 6 hours per device (15 minutes on Scan now), and at most 100 connections a second. It connects and closes at once, sending nothing, but security tools may still see it as a port scan: ask before turning it on for company networks." />
          </FormSection>
        )}
        <Field label="Description" htmlFor="sg-desc" error={errors.description?.message}><Textarea id="sg-desc" {...register('description')} /></Field>
        <FormError error={error} />
      </form>
    </Modal>
  )
}
