import { ChevronDown, Loader2 } from 'lucide-react'
import { type ButtonHTMLAttributes, cloneElement, Fragment, type InputHTMLAttributes, isValidElement, type ReactNode, type Ref, type SelectHTMLAttributes, useState } from 'react'
import { cx } from './cx'

type Variant = 'primary' | 'secondary' | 'danger' | 'ghost'

const variants: Record<Variant, string> = {
  primary: 'bg-accent text-white hover:bg-accent-hover border-transparent',
  secondary: 'bg-surface text-ink border-line-strong hover:bg-surface-2',
  danger: 'bg-danger text-white border-transparent hover:opacity-90',
  ghost: 'bg-transparent text-ink-2 border-transparent hover:bg-surface-2 hover:text-ink',
}

export function Button({ variant = 'secondary', busy, className, children, disabled, ...rest }:
  ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; busy?: boolean }) {
  return (
    <button
      type="button"
      {...rest}
      disabled={disabled || busy}
      className={cx('inline-flex h-8 cursor-pointer items-center justify-center gap-1.5 rounded-[5px] border px-3 text-[13px] font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-55', variants[variant], className)}
    >
      {busy && <Loader2 size={15} className="animate-spin" aria-hidden />}
      {children}
    </button>
  )
}

const control = 'h-8 w-full rounded-[5px] border border-line-strong bg-surface-2 px-2.5 text-[13px] text-ink placeholder:text-ink-3 focus:border-accent focus:bg-surface focus:outline-none aria-invalid:border-danger'

export function Input({ className, ref, ...rest }: InputHTMLAttributes<HTMLInputElement> & { ref?: Ref<HTMLInputElement> }) {
  return <input ref={ref} {...rest} className={cx(control, className)} />
}

export function Select({ className, ref, ...rest }: SelectHTMLAttributes<HTMLSelectElement> & { ref?: Ref<HTMLSelectElement> }) {
  return <select ref={ref} {...rest} className={cx(control, 'pr-7', className)} />
}

/** Ties the message to its control, so screen readers announce it. */
export function Field({ label, error, hint, htmlFor, children }:
  { label: string; error?: string; hint?: string; htmlFor: string; children: ReactNode }) {
  const noteId = `${htmlFor}-note`
  const note = error || hint ? noteId : undefined
  const linked = isValidElement<Record<string, unknown>>(children) && children.type !== Fragment
    ? cloneElement(children, { 'aria-invalid': Boolean(error) || undefined, 'aria-describedby': note })
    : children
  return (
    <div className="space-y-1">
      <label htmlFor={htmlFor} className="block text-[13px] font-medium text-ink">{label}</label>
      {linked}
      {error ? <p id={noteId} role="alert" className="text-[13px] text-danger">{error}</p>
        : hint && <p id={noteId} className="text-[13px] text-ink-3">{hint}</p>}
    </div>
  )
}

/**
 * Keeps less-used fields out of the way. Fields stay mounted, so a form still holds
 * their values. forceOpen shows them when one has an error, so nothing fails unseen.
 */
export function MoreOptions({ label = 'More options', forceOpen, children }: { label?: string; forceOpen?: boolean; children: ReactNode }) {
  const [open, setOpen] = useState(false)
  const shown = open || Boolean(forceOpen)
  return (
    <div>
      <button type="button" aria-expanded={shown} onClick={() => setOpen(!shown)} className="flex cursor-pointer items-center gap-1 text-[13px] font-medium text-accent hover:underline">
        <ChevronDown size={14} aria-hidden className={shown ? 'rotate-180' : undefined} /> {label}
      </button>
      <div hidden={!shown} className="mt-3">{children}</div>
    </div>
  )
}
