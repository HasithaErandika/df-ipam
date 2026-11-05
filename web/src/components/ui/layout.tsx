import { type ReactNode } from 'react'
import { type Severity } from '../../lib/format'
import { cx } from './cx'

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
