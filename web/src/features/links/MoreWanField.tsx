import { Plus, X } from 'lucide-react'
import { Button, Input, SearchSelect } from '../../components/ui'
import type { LinkOptions } from '../../lib/types'
import { inNetwork, networkOf, splitAddress } from '../../lib/validation'
import type { SectionProps } from './formParts'

/** One more WAN address: its subnet (a network such as 122.255.35.1/32) and the address. */
export interface WanRow { subnet: string; ip: string }

/** Stored "122.255.35.1/32" as a form row. */
export const wanRowOf = (stored: string): WanRow => {
  const [ip, bits] = stored.split('/')
  return { ip, subnet: networkOf(ip, Number(bits ?? 32)) }
}

/** A form row as the API takes it: the address with its subnet's prefix. */
export function wanRowValue(r: WanRow): string {
  const a = splitAddress(r.ip)
  if (!a) return r.ip.trim()
  const bits = a.bits ?? (r.subnet ? Number(r.subnet.split('/')[1]) : 32)
  return `${a.ip}/${bits}`
}

/** A row whose address isn't inside its subnet. */
export const wanRowIssue = (r: WanRow): string | undefined => {
  const a = splitAddress(r.ip)
  if (r.ip.trim() && !a) return `${r.ip.trim()} isn't an IPv4 address.`
  if (a && r.subnet && !inNetwork(a.ip, r.subnet)) return `${a.ip} isn't in ${r.subnet}.`
  return undefined
}

/**
 * More WAN addresses on the same link, such as an ILL's second public /32.
 * Each row is a subnet, then the address in it, like the main WAN address.
 */
export function MoreWanField({ p, options }: { p: SectionProps; options?: LinkOptions }) {
  const { f, set, errors } = p
  const segments = options?.segments ?? []
  const rows = f.extraWan
  const change = (i: number, patch: Partial<WanRow>) => set('extraWan', rows.map((r, j) => (i === j ? { ...r, ...patch } : r)))
  const tidy = (i: number) => {
    const a = splitAddress(rows[i].ip)
    if (a) change(i, { ip: a.ip, subnet: rows[i].subnet || networkOf(a.ip, a.bits ?? 32) })
  }
  const choices = (subnet: string) => [
    ...segments.map((s) => ({ value: s.detail, label: s.label, detail: s.detail })),
    ...(subnet && !segments.some((s) => s.detail === subnet) ? [{ value: subnet, label: subnet, detail: 'Not a segment' }] : []),
  ]
  return (
    <div className="space-y-2 sm:col-span-2 lg:col-span-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-[13px] font-medium text-ink">More WAN addresses</p>
        <Button onClick={() => set('extraWan', [...rows, { subnet: '', ip: '' }])}><Plus size={14} aria-hidden />Add WAN address</Button>
      </div>
      {rows.length === 0 && <p className="text-[13px] text-ink-3">For a link with more than one WAN address, such as an ILL with two public /32s.</p>}
      {rows.map((r, i) => (
        <div key={i} className="grid gap-2 sm:grid-cols-[1fr_1fr_auto]">
          <SearchSelect id={`link-extraWan-subnet-${i}`} value={r.subnet} onChange={(v) => change(i, { subnet: v })} options={choices(r.subnet)} emptyLabel="Subnet: not in the register" />
          <Input aria-label={`WAN address ${i + 2}`} aria-invalid={Boolean(wanRowIssue(r)) || undefined} className="font-mono" placeholder="122.255.35.1"
            value={r.ip} onChange={(e) => change(i, { ip: e.target.value })} onBlur={() => tidy(i)} />
          <Button variant="ghost" aria-label="Remove" onClick={() => set('extraWan', rows.filter((_, j) => j !== i))}><X size={15} /></Button>
          {wanRowIssue(r) && <p role="alert" className="text-[13px] text-danger sm:col-span-3">{wanRowIssue(r)}</p>}
        </div>
      ))}
      {errors.extraWanIps && <p role="alert" className="text-[13px] text-danger">{errors.extraWanIps}</p>}
    </div>
  )
}
