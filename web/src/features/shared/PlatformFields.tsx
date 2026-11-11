import { PlatformIcon, PlatformLabel } from '../../components/PlatformIcon'
import { Field, Select } from '../../components/ui'
import { platforms, services } from '../../lib/platforms'

/** The operating system, with its mark beside the choice and an offer to use the likely one. */
export function PlatformSelect({ id, value, onChange, guess }: { id: string; value: string; onChange: (v: string) => void; guess?: string | null }) {
  return (
    <Field label="Platform" htmlFor={id} hint={!value && guess ? undefined : 'The operating system or appliance family.'}>
      <div>
        <div className="flex items-center gap-2">
          <span className="grid size-8 shrink-0 place-items-center rounded-[5px] border border-line bg-surface"><PlatformIcon platform={value || null} size={18} /></span>
          <Select id={id} value={value} onChange={(e) => onChange(e.target.value)}>
            <option value="">Not recorded</option>
            {Object.entries(platforms).map(([k, v]) => <option key={k} value={k}>{v}</option>)}
          </Select>
        </div>
        {!value && guess && (
          <button type="button" onClick={() => onChange(guess)} className="mt-1 inline-flex cursor-pointer items-center gap-1.5 text-[12.5px] text-accent hover:underline">
            <PlatformIcon platform={guess} size={13} guessed />Use likely platform: {platforms[guess]}
          </button>
        )}
      </div>
    </Field>
  )
}

/** What a server mainly runs, as toggle chips. */
export function ServicePicker({ value, onChange }: { value: string[]; onChange: (v: string[]) => void }) {
  const toggle = (s: string) => onChange(value.includes(s) ? value.filter((x) => x !== s) : value.length >= 10 ? value : [...value, s])
  return (
    <fieldset>
      <legend className="mb-1.5 text-[13px] font-medium text-ink">Services <span className="font-normal text-ink-3">(optional, up to 10)</span></legend>
      <div className="flex flex-wrap gap-1.5">
        {Object.keys(services).map((s) => {
          const on = value.includes(s)
          return (
            <button key={s} type="button" role="checkbox" aria-checked={on} onClick={() => toggle(s)}
              className={`inline-flex h-7 cursor-pointer items-center rounded-full border px-2.5 text-[12.5px] ${on ? 'border-accent bg-accent-soft text-accent' : 'border-line-strong bg-surface text-ink-2 hover:border-ink-3'}`}>
              <PlatformLabel code={s} />
            </button>
          )
        })}
      </div>
    </fieldset>
  )
}
