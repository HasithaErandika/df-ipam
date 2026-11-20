import { useState } from 'react'
import { Field, Input } from '../../components/ui'
import type { LinkOptions } from '../../lib/types'
import { inNetwork, networkOf, splitAddress } from '../../lib/validation'
import type { SectionProps } from './formParts'

/** The register segment for an address: the exact network when a prefix is given, else the one holding it. */
export function segmentFor(segments: LinkOptions['segments'], ip: string, bits: number | null) {
  return bits == null ? segments.find((s) => inNetwork(ip, s.detail)) : segments.find((s) => s.detail === networkOf(ip, bits))
}

/**
 * The WAN IP, typed as the site diagrams write it: "10.10.40.253 /24" or
 * "122.255.4.226 255.255.255.248". Leaving the field keeps the address and
 * picks the matching segment as the WAN subnet when none is chosen.
 */
export function WanIpField({ p, options }: { p: SectionProps; options?: LinkOptions }) {
  const { f, set, errors } = p
  const [note, setNote] = useState('')
  const tidy = () => {
    const a = splitAddress(f.wanIp)
    if (!a) { setNote(''); return }
    set('wanIp', a.ip)
    const seg = segmentFor(options?.segments ?? [], a.ip, a.bits)
    if (seg && !f.wanSegmentId) set('wanSegmentId', seg.id)
    if (seg) setNote(`In ${seg.label} (${seg.detail}).`)
    else if (a.bits != null) setNote(`${networkOf(a.ip, a.bits)} isn't a segment yet. Add it in Network setup > Segments (a /32 works for one address), then pick it as the WAN subnet.`)
    else setNote('')
  }
  return (
    <>
      <Field label="WAN IP address" htmlFor="link-wanIp" error={errors.wanIp} hint={note || 'With its prefix or mask if you have it: 10.10.40.253 /24.'}>
        <Input id="link-wanIp" className="font-mono" list="link-register-ips" placeholder="10.10.40.253 /24" value={f.wanIp}
          onChange={(e) => set('wanIp', e.target.value)} onBlur={tidy} />
      </Field>
      <datalist id="link-register-ips">{options?.registerIps.map((o) => <option key={o.id} value={o.id}>{o.label}</option>)}</datalist>
    </>
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
