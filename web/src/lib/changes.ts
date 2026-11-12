/** A value as people read it in a list of changes. */
function shown(v: unknown): string {
  if (v === '' || v === null || v === undefined) return 'none'
  if (typeof v === 'boolean') return v ? 'on' : 'off'
  if (Array.isArray(v)) return v.length ? v.join(', ') : 'none'
  return String(v)
}

/**
 * The fields that differ between two versions of a form, one line each, such as
 * "Gateway: none → 192.168.1.1". Fields without a label are left out.
 */
export function listChanges<T extends object>(before: T, after: T, labels: Partial<Record<keyof T, string>>, show?: Partial<Record<keyof T, (v: unknown) => string>>): string[] {
  const out: string[] = []
  for (const key of Object.keys(labels) as (keyof T)[]) {
    const a = before[key]
    const b = after[key]
    if (JSON.stringify(a ?? '') === JSON.stringify(b ?? '')) continue
    const fmt = show?.[key] ?? shown
    out.push(`${labels[key]}: ${fmt(a)} → ${fmt(b)}`)
  }
  return out
}
