import type { ReactNode } from 'react'
import { Field, Input, Select, Textarea } from '../../components/ui'
import type { LinkForm } from './linkForm'

export type SetField = <K extends keyof LinkForm>(key: K, value: LinkForm[K]) => void

export interface SectionProps { f: LinkForm; set: SetField; errors: Record<string, string> }

/** A titled group of fields in the link form. */
export function FormBlock({ title, hint, children }: { title: string; hint?: ReactNode; children: ReactNode }) {
  return (
    <section className="rounded-md border border-line bg-surface">
      <header className="border-b border-line px-4 py-2.5">
        <h2 className="text-[13px] font-semibold text-ink">{title}</h2>
        {hint && <p className="text-[12px] text-ink-3">{hint}</p>}
      </header>
      <div className="grid gap-3 p-4 sm:grid-cols-2 lg:grid-cols-3">{children}</div>
    </section>
  )
}

const req = (label: string, required?: boolean) => (required ? `${label} *` : label)

type TextKey = { [K in keyof LinkForm]: LinkForm[K] extends string ? K : never }[keyof LinkForm]

/** A text input bound to one form field. error names the API field when it differs. */
export function TextField({ p, k, label, required, hint, error, type, placeholder, mono, wide, list }: {
  p: SectionProps; k: TextKey; label: string; required?: boolean; hint?: ReactNode; error?: string; type?: string
  placeholder?: string; mono?: boolean; wide?: boolean; list?: string
}) {
  const id = `link-${k}`
  return (
    <div className={wide ? 'sm:col-span-2 lg:col-span-3' : undefined}>
      <Field label={req(label, required)} htmlFor={id} hint={hint} error={p.errors[error ?? k]}>
        <Input id={id} type={type} list={list} placeholder={placeholder} className={mono ? 'font-mono' : undefined} value={p.f[k]}
          onChange={(e) => p.set(k, e.target.value)} />
      </Field>
    </div>
  )
}

export function AreaField({ p, k, label, required, hint, error, placeholder, mono }: {
  p: SectionProps; k: TextKey; label: string; required?: boolean; hint?: ReactNode; error?: string; placeholder?: string; mono?: boolean
}) {
  const id = `link-${k}`
  return (
    <div className="sm:col-span-2 lg:col-span-3">
      <Field label={req(label, required)} htmlFor={id} hint={hint} error={p.errors[error ?? k]}>
        <Textarea id={id} placeholder={placeholder} className={mono ? 'font-mono' : undefined} value={p.f[k]} onChange={(e) => p.set(k, e.target.value)} />
      </Field>
    </div>
  )
}

/** A dropdown from a fixed list. emptyLabel adds a "none" choice. */
export function ChoiceField({ p, k, label, choices, required, emptyLabel, hint }: {
  p: SectionProps; k: TextKey; label: string; choices: Record<string, string>; required?: boolean; emptyLabel?: string; hint?: ReactNode
}) {
  const id = `link-${k}`
  return (
    <Field label={req(label, required)} htmlFor={id} hint={hint} error={p.errors[k]}>
      <Select id={id} value={p.f[k]} onChange={(e) => p.set(k, e.target.value)}>
        {emptyLabel !== undefined && <option value="">{emptyLabel}</option>}
        {Object.entries(choices).map(([v, t]) => <option key={v} value={v}>{t}</option>)}
      </Select>
    </Field>
  )
}

/** A number with Mbps or Gbps. */
export function BandwidthField({ p, k, unit, label, error }: { p: SectionProps; k: 'down' | 'up'; unit: 'downUnit' | 'upUnit'; label: string; error: string }) {
  const id = `link-${k}`
  return (
    <Field label={label} htmlFor={id} error={p.errors[error]}>
      <div className="flex gap-2">
        <Input id={id} inputMode="decimal" value={p.f[k]} onChange={(e) => p.set(k, e.target.value)} />
        <Select aria-label={`${label} unit`} className="w-24" value={p.f[unit]} onChange={(e) => p.set(unit, e.target.value)}>
          <option>Mbps</option><option>Gbps</option>
        </Select>
      </div>
    </Field>
  )
}
