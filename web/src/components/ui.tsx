import * as Dialog from '@radix-ui/react-dialog'
import { AlertTriangle, ChevronDown, ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight, Inbox, Loader2, X } from 'lucide-react'
import { cloneElement, Component, Fragment, isValidElement, useState, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes, type Ref } from 'react'
import { ApiError, type PageMeta } from '../lib/api'
import { pageWindow, type Severity } from '../lib/format'

const cx = (...parts: (string | false | undefined)[]) => parts.filter(Boolean).join(' ')

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

/** Catches a crash in a page, so people get a way forward instead of a blank screen. */
export class ErrorBoundary extends Component<{ children: ReactNode; resetKey?: string }, { failed: boolean; key?: string }> {
  state = { failed: false, key: this.props.resetKey }

  static getDerivedStateFromError() { return { failed: true } }

  // Moving to another page clears the failure.
  static getDerivedStateFromProps(props: { resetKey?: string }, state: { failed: boolean; key?: string }) {
    return props.resetKey === state.key ? null : { failed: false, key: props.resetKey }
  }

  render() {
    if (!this.state.failed) return this.props.children
    return (
      <div role="alert" className="flex flex-col items-center p-10 text-center">
        <AlertTriangle size={26} className="mb-3 text-danger" aria-hidden />
        <h1 className="text-base font-semibold text-ink">This page ran into a problem</h1>
        <p className="mt-1 text-ink-2">Your data is safe. Reload the page, or go to another one.</p>
        <div className="mt-4"><Button variant="primary" onClick={() => window.location.reload()}>Reload</Button></div>
      </div>
    )
  }
}

export function Card({ title, action, children, className }:
  { title?: string; action?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={cx('rounded-md border border-line bg-surface', className)}>
      {title && (
        <header className="flex h-10 items-center justify-between border-b border-line px-4">
          <h2 className="text-[13px] font-semibold text-ink">{title}</h2>
          {action}
        </header>
      )}
      {children}
    </section>
  )
}

export function PageHeader({ title, description, actions }: { title: string; description?: string; actions?: ReactNode }) {
  return (
    <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
      <div>
        <h1 className="text-xl leading-tight font-semibold tracking-tight text-ink">{title}</h1>
        {description && <p className="mt-0.5 max-w-2xl text-[13px] text-ink-2">{description}</p>}
      </div>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </div>
  )
}

const tones = {
  neutral: 'bg-surface-2 text-ink-2 border-line',
  accent: 'bg-accent-soft text-accent border-transparent',
  ok: 'bg-ok-soft text-ok border-transparent',
  warn: 'bg-warn-soft text-warn border-transparent',
  danger: 'bg-danger-soft text-danger border-transparent',
}

export function Badge({ tone = 'neutral', children }: { tone?: keyof typeof tones; children: ReactNode }) {
  return <span className={cx('inline-flex items-center rounded-[3px] border px-1.5 text-[11px] leading-[18px] font-medium whitespace-nowrap', tones[tone])}>{children}</span>
}

export function Mono({ children }: { children: ReactNode }) {
  return <span className="font-mono text-[13px]">{children}</span>
}

const meterFill: Record<Severity, string> = { normal: 'bg-accent', warning: 'bg-[var(--meter-warning)]', critical: 'bg-danger' }

export function Meter({ pct, level, label }: { pct: number; level: Severity; label: string }) {
  return (
    <div className="flex items-center gap-2">
      <div role="meter" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100} aria-label={label}
        className="h-1.5 w-20 overflow-hidden rounded-full bg-track">
        <div className={cx('h-full rounded-full', meterFill[level])} style={{ width: `${Math.min(pct, 100)}%` }} />
      </div>
      <span className="w-12 text-right text-[13px] text-ink-2 tabular-nums">{pct.toFixed(1)}%</span>
    </div>
  )
}

export function Th({ children, className }: { children?: ReactNode; className?: string }) {
  return <th scope="col" className={cx('border-b border-line bg-surface-2 px-3 py-1.5 text-left text-[11px] font-semibold tracking-[.04em] text-ink-3 uppercase', className)}>{children}</th>
}

export function Td({ children, className }: { children?: ReactNode; className?: string }) {
  return <td className={cx('border-b border-line px-3 py-[7px] align-middle text-[13px]', className)}>{children}</td>
}

export function SkeletonRows({ rows = 6 }: { rows?: number }) {
  return (
    <div className="space-y-2 p-4" aria-label="Loading" role="status">
      {Array.from({ length: rows }, (_, i) => <div key={i} className="h-6 animate-pulse rounded bg-track" />)}
    </div>
  )
}

export function EmptyState({ title, body, action }: { title: string; body?: string; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center px-6 py-14 text-center">
      <Inbox size={26} className="mb-2.5 text-ink-3" aria-hidden />
      <p className="font-medium text-ink">{title}</p>
      {body && <p className="mt-1 max-w-md text-ink-2">{body}</p>}
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}

export function ErrorState({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  const api = error instanceof ApiError ? error : undefined
  return (
    <div role="alert" className="flex flex-col items-center px-6 py-14 text-center">
      <AlertTriangle size={26} className="mb-2.5 text-danger" aria-hidden />
      <p className="font-medium text-ink">{api?.message ?? 'Something went wrong.'}</p>
      {api?.correlationId && <p className="mt-1 text-[13px] text-ink-3">Reference: <Mono>{api.correlationId}</Mono></p>}
      {onRetry && <Button className="mt-4" onClick={onRetry}>Try again</Button>}
    </div>
  )
}

export function FormError({ error }: { error: unknown }) {
  if (!error) return null
  return (
    <div role="alert" className="flex gap-2 rounded-md bg-danger-soft px-3 py-2 text-danger">
      <AlertTriangle size={16} className="mt-0.5 shrink-0" aria-hidden />
      <span>{error instanceof Error ? error.message : 'Something went wrong.'}</span>
    </div>
  )
}

const pageSizes = [25, 50, 100, 200]

const pageButton = 'grid h-7 min-w-7 cursor-pointer place-items-center rounded-[4px] px-1 text-[12.5px] tabular-nums text-ink-2 hover:bg-surface-2 hover:text-ink disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent'

export function Pagination({ meta, onPage, onPageSize }:
  { meta: PageMeta; onPage: (page: number) => void; onPageSize?: (size: number) => void }) {
  const { page, pageSize, total, totalPages, hasNext, hasPrev } = meta
  const first = total === 0 ? 0 : (page - 1) * pageSize + 1
  return (
    <nav aria-label="Pagination" className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 border-t border-line px-3 py-1.5 text-[12.5px] text-ink-2">
      <div className="flex items-center gap-4">
        <span className="tabular-nums">{first.toLocaleString('en-GB')}–{Math.min(page * pageSize, total).toLocaleString('en-GB')} of {total.toLocaleString('en-GB')}</span>
        {onPageSize && (
          <label className="flex items-center gap-1.5">
            Rows
            <select aria-label="Rows per page" value={pageSize} onChange={(e) => onPageSize(Number(e.target.value))}
              className="h-7 cursor-pointer rounded-[4px] border border-line bg-surface px-1 text-[12.5px] text-ink focus:border-accent focus:outline-none">
              {pageSizes.map((size) => <option key={size} value={size}>{size}</option>)}
            </select>
          </label>
        )}
      </div>
      <div className="flex items-center gap-0.5">
        <button type="button" className={pageButton} aria-label="First page" disabled={!hasPrev} onClick={() => onPage(1)}><ChevronsLeft size={15} /></button>
        <button type="button" className={pageButton} aria-label="Previous page" disabled={!hasPrev} onClick={() => onPage(page - 1)}><ChevronLeft size={15} /></button>
        {pageWindow(page, totalPages).map((p, i) => p === null
          ? <span key={`gap-${i}`} aria-hidden className="w-6 text-center text-ink-3">…</span>
          : (
            <button key={p} type="button" aria-label={`Page ${p}`} aria-current={p === page ? 'page' : undefined} onClick={() => onPage(p)}
              className={p === page ? `${pageButton} bg-accent font-medium text-white hover:bg-accent hover:text-white` : pageButton}>{p}</button>
          ))}
        <button type="button" className={pageButton} aria-label="Next page" disabled={!hasNext} onClick={() => onPage(page + 1)}><ChevronRight size={15} /></button>
        <button type="button" className={pageButton} aria-label="Last page" disabled={!hasNext} onClick={() => onPage(totalPages)}><ChevronsRight size={15} /></button>
      </div>
    </nav>
  )
}

const sizes = { md: 'max-w-lg', lg: 'max-w-2xl', xl: 'max-w-4xl' }

const backdrop = 'overlay fixed inset-0 grid place-items-center overflow-y-auto bg-[#0b1220]/35 p-4 backdrop-blur-[6px]'

export function Modal({ open, onClose, title, subtitle, children, footer, size = 'md' }:
  { open: boolean; onClose: () => void; title: string; subtitle?: ReactNode; children: ReactNode; footer?: ReactNode; size?: keyof typeof sizes }) {
  return (
    <Dialog.Root open={open} onOpenChange={(next) => !next && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className={cx(backdrop, 'z-40')}>
          <Dialog.Content aria-describedby={undefined} className={cx('modal flex max-h-[calc(100vh-2rem)] w-full flex-col overflow-hidden rounded-lg border border-line-strong/70 bg-surface shadow-[0_24px_64px_-16px_rgba(11,18,32,.35)]', sizes[size])}>
            <header className="flex items-start justify-between gap-3 border-b border-line px-5 py-3.5">
              <div className="min-w-0">
                <Dialog.Title className="truncate text-[15px] font-semibold text-ink">{title}</Dialog.Title>
                {subtitle && <div className="mt-0.5 text-[13px] text-ink-2">{subtitle}</div>}
              </div>
              <Dialog.Close asChild>
                <Button variant="ghost" className="h-7 px-1" aria-label="Close"><X size={17} /></Button>
              </Dialog.Close>
            </header>
            <div className="flex-1 overflow-y-auto px-5 py-4">{children}</div>
            {footer && <footer className="flex justify-end gap-2 border-t border-line bg-surface-2 px-5 py-2.5">{footer}</footer>}
          </Dialog.Content>
        </Dialog.Overlay>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

export function ConfirmDialog({ open, title, body, confirmLabel, busy, error, onConfirm, onCancel }:
  { open: boolean; title: string; body: string; confirmLabel: string; busy?: boolean; error?: unknown; onConfirm: () => void; onCancel: () => void }) {
  return (
    <Dialog.Root open={open} onOpenChange={(next) => !next && onCancel()}>
      <Dialog.Portal>
        <Dialog.Overlay className={cx(backdrop, 'z-[60]')}>
          <Dialog.Content className="modal w-full max-w-md rounded-lg border border-line-strong/70 bg-surface p-5 shadow-[0_24px_64px_-16px_rgba(11,18,32,.35)]">
            <Dialog.Title className="text-[15px] font-semibold text-ink">{title}</Dialog.Title>
            <Dialog.Description className="mt-1.5 text-ink-2">{body}</Dialog.Description>
            {error ? <div className="mt-3"><FormError error={error} /></div> : null}
            <div className="mt-5 flex justify-end gap-2">
              <Button onClick={onCancel}>Cancel</Button>
              <Button variant="danger" busy={busy} onClick={onConfirm}>{confirmLabel}</Button>
            </div>
          </Dialog.Content>
        </Dialog.Overlay>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
