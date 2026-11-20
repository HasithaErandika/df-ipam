import { Field, Input, SearchSelect } from '../../components/ui'
import type { LinkOptions, LinkProvider, LinkSettings } from '../../lib/types'
import { criticalities, linkEnvironments, linkRoles, linkStatuses, mediums, routings } from './labels'
import { BandwidthField, ChoiceField, FormBlock, type SectionProps, TextField } from './formParts'

const opts = (list: { id: string; label: string; detail: string }[] = []) => list.map((o) => ({ value: o.id, label: o.label, detail: o.detail }))

/** Section A: what the link is, where it runs and who answers for it. */
export function BasicSection({ p, settings, providers, options, editing }: {
  p: SectionProps; settings?: LinkSettings; providers: LinkProvider[]; options?: LinkOptions; editing: boolean
}) {
  const { f, set, errors } = p
  // A switched-off type or provider stays selectable on the link that already has it.
  const types = Object.fromEntries((settings?.types ?? []).filter((t) => t.active || t.key === f.linkType).map((t) => [t.key, t.label]))
  const sites = Object.fromEntries([...(settings?.sites ?? []), ...(f.sourceSite && !settings?.sites.includes(f.sourceSite) ? [f.sourceSite] : [])].map((x) => [x, x]))
  const providerOpts = providers.filter((x) => x.active || x.id === f.providerId).map((x) => ({ value: x.id, label: x.name, detail: x.active ? undefined : 'Inactive' }))
  return (
    <FormBlock title="Basic information" hint={editing ? undefined : 'The Link ID (DF-LNK-0001) is given when the link is saved.'}>
      <TextField p={p} k="name" label="Link name" required placeholder="Head Office Primary ILL – Dialog" />
      <ChoiceField p={p} k="linkType" label="Link type" required choices={types} emptyLabel="Choose…" />
      <Field label="Service provider" htmlFor="link-providerId" error={errors.providerId} hint="The provider gives the support: its contacts show on the link. Add providers in Service Providers.">
        <SearchSelect id="link-providerId" value={f.providerId} onChange={(v) => set('providerId', v)} options={providerOpts} emptyLabel="No provider" />
      </Field>
      <TextField p={p} k="circuitId" label="Circuit / service ID" mono />
      <ChoiceField p={p} k="sourceSite" label="Source location" required choices={sites} emptyLabel="Choose…" />
      <TextField p={p} k="destination" label="Destination" hint="A location, or an external party such as a partner's data centre." />
      <TextField p={p} k="purpose" label="Business purpose" required wide />
      <ChoiceField p={p} k="environment" label="Environment" choices={linkEnvironments} emptyLabel="Not set" />
      <ChoiceField p={p} k="linkRole" label="Link role" required choices={linkRoles} />
      <ChoiceField p={p} k="criticality" label="Criticality" required choices={criticalities} />
      <Field label="Responsible officer" htmlFor="link-officerId" error={errors.officerId} hint="Gets the contract expiry reminders.">
        <SearchSelect id="link-officerId" value={f.officerId} onChange={(v) => set('officerId', v)} options={opts(options?.officers)} emptyLabel="Nobody" />
      </Field>
      <ChoiceField p={p} k="status" label="Status" required choices={linkStatuses} hint="The administrative status, not whether the link is up now." />
    </FormBlock>
  )
}

/** Section B: how the link is built. Every field is optional. */
export function TechnicalSection({ p, options }: { p: SectionProps; options?: LinkOptions }) {
  const { f, set, errors } = p
  return (
    <FormBlock title="Technical information" hint="All optional. Addresses are IPv4.">
      <BandwidthField p={p} k="down" unit="downUnit" label="Download bandwidth" error="downMbps" />
      <BandwidthField p={p} k="up" unit="upUnit" label="Upload bandwidth" error="upMbps" />
      <ChoiceField p={p} k="medium" label="Connection medium" choices={mediums} emptyLabel="Not set" />
      <TextField p={p} k="wanIp" label="WAN IP address" mono list="link-register-ips" placeholder="Pick from the register or type one" />
      <datalist id="link-register-ips">{options?.registerIps.map((o) => <option key={o.id} value={o.id}>{o.label}</option>)}</datalist>
      <Field label="WAN subnet" htmlFor="link-wanSegmentId" error={errors.wanSegmentId}>
        <SearchSelect id="link-wanSegmentId" value={f.wanSegmentId} onChange={(v) => set('wanSegmentId', v)} options={opts(options?.segments)} emptyLabel="Not in the register" />
      </Field>
      <TextField p={p} k="publicIps" label="Public IP address / range" mono placeholder="203.0.113.0/29" />
      <TextField p={p} k="gatewayIp" label="Gateway IP" mono />
      <Field label="Connected device" htmlFor="link-deviceId" error={errors.deviceId} hint="An approved device, or type a name below.">
        <SearchSelect id="link-deviceId" value={f.deviceId} onChange={(v) => set('deviceId', v)} options={opts(options?.devices)} emptyLabel="Not in Devices" />
      </Field>
      {!f.deviceId && <TextField p={p} k="deviceText" label="Connected device (not in Devices)" placeholder="FW-01" />}
      <TextField p={p} k="interface" label="Interface / port" mono placeholder="port3" />
      <Field label="VLAN ID" htmlFor="link-vlan" error={errors.vlan}><Input id="link-vlan" inputMode="numeric" value={f.vlan} onChange={(e) => set('vlan', e.target.value)} /></Field>
      <TextField p={p} k="vrf" label="VDOM / VRF" />
      <ChoiceField p={p} k="routing" label="Routing type" choices={routings} emptyLabel="Not set" />
    </FormBlock>
  )
}
