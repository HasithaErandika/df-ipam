import * as DropdownMenu from '@radix-ui/react-dropdown-menu'
import { Check, ChevronDown } from 'lucide-react'
import { PlatformIcon, PlatformLabel } from '../../components/PlatformIcon'
import { Field } from '../../components/ui'
import { knownPlatform, platforms, services } from '../../lib/platforms'

const choices: [string, string][] = [['', 'Not recorded'], ...Object.entries(platforms)]

/**
 * The operating system, chosen from a list with each platform's mark, and an
 * offer to use the likely one. A menu rather than a native select: a long native
 * list inside a dialog can't be scrolled in some browsers, because the dialog
 * locks page scrolling. The menu takes the scroll lock over while it is open.
 */
export function PlatformSelect({ id, value: raw, onChange, guess }: { id: string; value: string; onChange: (v: string) => void; guess?: string | null }) {
  const value = knownPlatform(raw)
  return (
    <Field label="Platform" htmlFor={id} hint={!value && guess ? undefined : 'The operating system or appliance family.'}>
      <div>
        <DropdownMenu.Root>
          <DropdownMenu.Trigger id={id}
            className="flex h-8 w-full cursor-pointer items-center gap-2 rounded-[5px] border border-line-strong bg-surface-2 px-2.5 text-left text-[13px] text-ink outline-none focus:border-accent focus:bg-surface data-[state=open]:border-accent data-[state=open]:bg-surface">
            <PlatformIcon platform={value || null} size={16} />
            <span className={`min-w-0 flex-1 truncate ${value ? '' : 'text-ink-3'}`}>{value ? platforms[value] : 'Not recorded'}</span>
            <ChevronDown size={15} className="shrink-0 text-ink-3" aria-hidden />
          </DropdownMenu.Trigger>
          <DropdownMenu.Portal>
            <DropdownMenu.Content align="start" sideOffset={4} collisionPadding={8}
              className="z-50 max-h-[min(18rem,var(--radix-dropdown-menu-content-available-height))] w-[var(--radix-dropdown-menu-trigger-width)] min-w-56 overflow-y-auto rounded-md border border-line-strong/70 bg-surface p-1 shadow-[0_12px_32px_-12px_rgba(11,18,32,.3)]">
              <DropdownMenu.RadioGroup value={value} onValueChange={onChange}>
                {choices.map(([code, name]) => (
                  <DropdownMenu.RadioItem key={code || 'none'} value={code}
                    className="flex h-8 cursor-pointer items-center gap-2 rounded-[4px] px-2 text-[13px] text-ink-2 outline-none data-[highlighted]:bg-bg data-[highlighted]:text-ink data-[state=checked]:font-medium data-[state=checked]:text-ink">
                    <span className="grid size-4 shrink-0 place-items-center">{code && <PlatformIcon platform={code} size={14} />}</span>
                    <span className="min-w-0 flex-1 truncate">{name}</span>
                    <DropdownMenu.ItemIndicator><Check size={14} className="text-accent" aria-hidden /></DropdownMenu.ItemIndicator>
                  </DropdownMenu.RadioItem>
                ))}
              </DropdownMenu.RadioGroup>
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>
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
