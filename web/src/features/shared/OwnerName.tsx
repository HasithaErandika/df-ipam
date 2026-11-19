import type { ReactNode } from 'react'

/**
 * A device's name led by its owner when one is recorded: "Nimal Perera › DESKTOP-7Q2".
 * Without an owner it is just the name, or the fallback when there is none.
 */
export function OwnerName({ owner, name, fallback }: { owner?: string | null; name?: string | null; fallback: ReactNode }) {
  const who = owner?.trim()
  if (!who) return <>{name || fallback}</>
  return (
    <span title={`${who} › ${name || 'no name'}`}>
      <span className="font-medium text-ink">{who}</span>
      <span aria-hidden className="mx-1 text-ink-3">›</span>
      <span className="sr-only">, device </span>
      {name || fallback}
    </span>
  )
}
