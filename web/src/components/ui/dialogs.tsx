import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import { type ReactNode } from 'react'
import { Button } from './controls'
import { cx } from './cx'
import { FormError } from './feedback'

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

export function ConfirmDialog({ open, title, body, confirmLabel, busy, error, tone = 'danger', onConfirm, onCancel }:
  { open: boolean; title: string; body: string; confirmLabel: string; busy?: boolean; error?: unknown; tone?: 'danger' | 'primary'; onConfirm: () => void; onCancel: () => void }) {
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
              <Button variant={tone} busy={busy} onClick={onConfirm}>{confirmLabel}</Button>
            </div>
          </Dialog.Content>
        </Dialog.Overlay>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
