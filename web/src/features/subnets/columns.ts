import { useState } from 'react'

export const columnLabels = { routingDomain: 'Routing domain', environment: 'Environment', site: 'Site', vlan: 'VLAN', gateway: 'Gateway', usable: 'Usable', utilisation: 'Utilisation' }

export type Column = keyof typeof columnLabels

export function useColumns(): [Set<Column>, (c: Column) => void] {
  const [shown, setShown] = useState<Set<Column>>(() => {
    try {
      const stored = JSON.parse(localStorage.getItem('dfipam.subnetColumns') ?? 'null')
      if (Array.isArray(stored)) return new Set(stored as Column[])
    } catch { /* fall back to defaults */ }
    return new Set<Column>(['routingDomain', 'environment', 'site', 'gateway', 'usable', 'utilisation'])
  })
  const toggle = (c: Column) => setShown((prev) => {
    const next = new Set(prev)
    if (next.has(c)) next.delete(c)
    else next.add(c)
    try { localStorage.setItem('dfipam.subnetColumns', JSON.stringify([...next])) } catch { /* not stored */ }
    return next
  })
  return [shown, toggle]
}
