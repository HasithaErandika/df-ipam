import { AlertTriangle, CheckCircle2, X } from 'lucide-react'
import { useSyncExternalStore } from 'react'

type Kind = 'success' | 'error'
interface Item { id: number; kind: Kind; message: string }

const lifetime: Record<Kind, number> = { success: 4000, error: 8000 }
let items: Item[] = []
let nextId = 1
const listeners = new Set<() => void>()

function publish(next: Item[]) {
  items = next
  for (const listener of listeners) listener()
}

function dismiss(id: number) {
  publish(items.filter((t) => t.id !== id))
}

function show(kind: Kind, message: string) {
  // One failure can reach several screens at once, and one message is enough.
  if (items.some((t) => t.kind === kind && t.message === message)) return
  const id = nextId++
  publish([...items, { id, kind, message }])
  setTimeout(() => dismiss(id), lifetime[kind])
}

/** Short messages about what just happened. They work outside React too. */
export const toast = {
  success: (message: string) => show('success', message),
  error: (message: string) => show('error', message),
}

const subscribe = (listener: () => void) => {
  listeners.add(listener)
  return () => { listeners.delete(listener) }
}

export function Toaster() {
  const shown = useSyncExternalStore(subscribe, () => items)
  return (
    <div aria-live="polite" className="pointer-events-none fixed right-4 bottom-4 z-[70] flex w-full max-w-sm flex-col gap-2">
      {shown.map((t) => (
        <div key={t.id} role={t.kind === 'error' ? 'alert' : 'status'}
          className="pointer-events-auto flex items-start gap-2.5 rounded-md border border-line-strong/70 bg-surface px-3 py-2.5 text-[13px] text-ink shadow-[0_12px_32px_-12px_rgba(11,18,32,.35)]">
          {t.kind === 'error'
            ? <AlertTriangle size={16} className="mt-0.5 shrink-0 text-danger" aria-hidden />
            : <CheckCircle2 size={16} className="mt-0.5 shrink-0 text-ok" aria-hidden />}
          <span className="min-w-0 flex-1 break-words">{t.message}</span>
          <button type="button" aria-label="Dismiss" className="cursor-pointer text-ink-3 hover:text-ink" onClick={() => dismiss(t.id)}><X size={14} aria-hidden /></button>
        </div>
      ))}
    </div>
  )
}
