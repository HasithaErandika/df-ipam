import { type ReactNode, type Ref, type TextareaHTMLAttributes } from 'react'
import { cx } from './cx'

export function Textarea({ className, ref, ...rest }: TextareaHTMLAttributes<HTMLTextAreaElement> & { ref?: Ref<HTMLTextAreaElement> }) {
  return <textarea ref={ref} rows={3} {...rest}
    className={cx('w-full rounded-[5px] border border-line-strong bg-surface-2 px-2.5 py-1.5 text-[13px] text-ink placeholder:text-ink-3 focus:border-accent focus:bg-surface focus:outline-none aria-invalid:border-danger', className)} />
}

/** An on/off switch with its label and an optional line of help. */
export function Switch({ checked, onChange, label, hint, disabled, id }: {
  checked: boolean; onChange: (v: boolean) => void; label: ReactNode; hint?: ReactNode; disabled?: boolean; id: string
}) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="min-w-0">
        <label htmlFor={id} className="block cursor-pointer text-[13px] font-medium text-ink">{label}</label>
        {hint && <p className="mt-0.5 text-[12.5px] text-ink-3">{hint}</p>}
      </div>
      <button id={id} type="button" role="switch" aria-checked={checked} disabled={disabled} onClick={() => onChange(!checked)}
        className={cx('relative mt-0.5 h-5 w-9 shrink-0 cursor-pointer rounded-full transition-colors disabled:cursor-not-allowed disabled:opacity-50',
          checked ? 'bg-accent' : 'bg-line-strong')}>
        <span aria-hidden className={cx('absolute top-0.5 left-0.5 size-4 rounded-full bg-white shadow transition-transform', checked && 'translate-x-4')} />
      </button>
    </div>
  )
}

/** A section heading inside a form, to group related fields. */
export function FormSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <fieldset className="space-y-3">
      <legend className="mb-2 text-[11px] font-semibold tracking-[.06em] text-ink-3 uppercase">{title}</legend>
      {children}
    </fieldset>
  )
}

/** A key figure with a label, for summaries at the top of a screen. */
export function Stat({ label, value, tone, hint, onClick }: { label: string; value: ReactNode; tone?: 'danger' | 'warn' | 'ok'; hint?: ReactNode; onClick?: () => void }) {
  const toneClass = tone === 'danger' ? 'text-danger' : tone === 'warn' ? 'text-warn' : tone === 'ok' ? 'text-ok' : 'text-ink'
  const body = (
    <>
      <div className="text-[12px] font-medium text-ink-3">{label}</div>
      <div className={cx('mt-1 text-[22px] leading-none font-semibold tabular-nums', toneClass)}>{value}</div>
      {hint && <div className="mt-1.5 text-[12px] text-ink-3">{hint}</div>}
    </>
  )
  const box = 'rounded-md border border-line bg-surface px-4 py-3 text-left'
  return onClick
    ? <button type="button" onClick={onClick} className={cx(box, 'cursor-pointer transition-colors hover:border-accent/50 hover:bg-accent-soft/30')}>{body}</button>
    : <div className={box}>{body}</div>
}

/** Label and value pairs, for details in a dialog. */
export function Details({ items }: { items: [string, ReactNode][] }) {
  return (
    <dl className="grid grid-cols-2 gap-x-4 gap-y-3 max-sm:grid-cols-1">
      {items.map(([k, v]) => (
        <div key={k} className="min-w-0">
          <dt className="text-[12px] text-ink-3">{k}</dt>
          <dd className="mt-0.5 text-[13px] break-words text-ink">{v ?? '—'}</dd>
        </div>
      ))}
    </dl>
  )
}

/** A short notice inside a form or page. */
export function Notice({ tone = 'neutral', children }: { tone?: 'neutral' | 'warn' | 'danger' | 'ok'; children: ReactNode }) {
  const tones = { neutral: 'bg-surface-2 text-ink-2 border-line', warn: 'bg-warn-soft text-warn border-transparent', danger: 'bg-danger-soft text-danger border-transparent', ok: 'bg-ok-soft text-ok border-transparent' }
  return <div role={tone === 'danger' ? 'alert' : 'status'} className={cx('rounded-md border px-3 py-2 text-[13px]', tones[tone])}>{children}</div>
}
