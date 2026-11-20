import { useState } from 'react'
import { Field, Input, MultiSearchSelect, SearchSelect } from '../../components/ui'
import type { LinkOptions } from '../../lib/types'
import { inNetwork, networkOf, splitAddress } from '../../lib/validation'
import type { SectionProps } from './formParts'

/** The register segment for an address: the exact network when a prefix is given, else the one holding it. */
export function segmentFor(segments: LinkOptions['segments'], ip: string, bits: number | null) {
  return bits == null ? segments.find((s) => inNetwork(ip, s.detail)) : segments.find((s) => s.detail === networkOf(ip, bits))
}

const opts = (list: LinkOptions['segments'] = []) => list.map((o) => ({ value: o.id, label: o.label, detail: o.detail }))

/**
 * The WAN subnet, then the WAN IP inside it. Choosing the subnet narrows the
 * register addresses offered for the IP. The IP may be typed as the site
 * diagrams write it ("10.10.40.253 /24", "122.255.4.226 255.255.255.248"):
 * leaving the field keeps the address, and picks the matching segment when no
 * subnet is chosen yet.
 */
export function WanFields({ p, options }: { p: SectionProps; options?: LinkOptions }) {
  const { f, set, errors } = p
  const [note, setNote] = useState('')
  const segments = options?.segments ?? []
  const chosen = segments.find((s) => s.id === f.wanSegmentId)
  const ips = (options?.registerIps ?? []).filter((o) => !chosen || o.detail === chosen.id)
  const outside = (ip: string) => (chosen && !inNetwork(ip, chosen.detail) ? `${ip} isn't in ${chosen.label} (${chosen.detail}). Check the address or the subnet.` : '')
  const tidy = () => {
    const a = splitAddress(f.wanIp)
    if (!a) { setNote(''); return }
    set('wanIp', a.ip)
    if (chosen) { setNote(outside(a.ip)); return }
    const seg = segmentFor(segments, a.ip, a.bits)
    if (seg) { set('wanSegmentId', seg.id); setNote(`WAN subnet set to ${seg.label} (${seg.detail}).`) }
    else setNote(a.bits != null ? `${networkOf(a.ip, a.bits)} isn't a segment yet. Add it in Network setup > Segments (a /32 works for one address), then choose it as the WAN subnet.` : '')
  }
  const pick = (id: string) => {
    set('wanSegmentId', id)
    const seg = segments.find((s) => s.id === id)
    const a = splitAddress(f.wanIp)
    setNote(seg && a && !inNetwork(a.ip, seg.detail) ? `${a.ip} isn't in ${seg.label} (${seg.detail}). Check the address or the subnet.` : '')
  }
  return (
    <>
      <Field label="WAN subnet" htmlFor="link-wanSegmentId" error={errors.wanSegmentId} hint="Choose it first: the WAN IP is offered from it. A single address can be a /32 segment.">
        <SearchSelect id="link-wanSegmentId" value={f.wanSegmentId} onChange={pick} options={opts(segments)} emptyLabel="Not in the register" />
      </Field>
      <Field label="WAN IP address" htmlFor="link-wanIp" error={errors.wanIp}
        hint={note || (chosen ? `An address in ${chosen.detail}, from the register or typed.` : 'With its prefix or mask if you have it: 10.10.40.253 /24.')}>
        <Input id="link-wanIp" className="font-mono" list="link-register-ips" placeholder={chosen ? chosen.detail.replace(/\/\d+$/, '') : '10.10.40.253 /24'} value={f.wanIp}
          onChange={(e) => set('wanIp', e.target.value)} onBlur={tidy} />
      </Field>
      <datalist id="link-register-ips">{ips.map((o) => <option key={o.id} value={o.id}>{o.label}</option>)}</datalist>
    </>
  )
}

/** Every segment the link serves, for any link type: the LANs behind it, a VPN's local subnets. */
export function SegmentsField({ p, options }: { p: SectionProps; options?: LinkOptions }) {
  const { f, set, errors } = p
  const choices = opts(options?.segments).filter((o) => o.value !== f.wanSegmentId || f.localSegmentIds.includes(o.value))
  return (
    <div className="sm:col-span-2 lg:col-span-3">
      <Field label="Segments" htmlFor="link-localSegmentIds" error={errors.localSegmentIds}
        hint="Every segment this link serves or carries: the LANs behind it, a VPN's local subnets. Add as many as you need; the WAN subnet needn't be repeated.">
        <MultiSearchSelect id="link-localSegmentIds" values={f.localSegmentIds} onChange={(v) => set('localSegmentIds', v)} options={choices} placeholder="Add a segment from the register…" />
      </Field>
    </div>
  )
}

/** An address field that drops a prefix or mask typed after the address. */
export function HostField({ p, k, label, placeholder }: { p: SectionProps; k: 'gatewayIp'; label: string; placeholder?: string }) {
  const id = `link-${k}`
  const tidy = () => { const a = splitAddress(p.f[k]); if (a) p.set(k, a.ip) }
  return (
    <Field label={label} htmlFor={id} error={p.errors[k]}>
      <Input id={id} className="font-mono" placeholder={placeholder} value={p.f[k]} onChange={(e) => p.set(k, e.target.value)} onBlur={tidy} />
    </Field>
  )
}

/** Public IP or range. A dotted mask becomes a prefix: "122.255.4.224 255.255.255.248" is 122.255.4.224/29. */
export function PublicIpsField({ p }: { p: SectionProps }) {
  const tidy = () => {
    const a = splitAddress(p.f.publicIps)
    if (a) p.set('publicIps', a.bits == null || a.bits === 32 ? a.ip : `${a.ip}/${a.bits}`)
  }
  return (
    <Field label="Public IP address / range" htmlFor="link-publicIps" error={p.errors.publicIps} hint="One address, or the block as its network: 122.255.4.224/29.">
      <Input id="link-publicIps" className="font-mono" placeholder="203.0.113.0/29" value={p.f.publicIps} onChange={(e) => p.set('publicIps', e.target.value)} onBlur={tidy} />
    </Field>
  )
}
