import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router'
import { listAll } from './api'

export function useDebounced<T>(value: T, ms = 300): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => setDebounced(value), ms)
    return () => clearTimeout(id)
  }, [value, ms])
  return debounced
}

export interface Option { id: string; name: string; code?: string; vlanId?: number; cidr?: string; email?: string; displayName?: string; deviceName?: string; siteName?: string; reason?: string; expired?: boolean }

/** Loads every option for a dropdown. */
export const useOptions = (path: string, enabled = true) =>
  useQuery({ queryKey: ['options', path], enabled, queryFn: () => listAll<Option>(path), staleTime: 30_000 })

/** Readable label for an option, whatever kind of record it is. */
export function optionLabel(o: Option): string {
  if (o.vlanId !== undefined) return `${o.vlanId}${o.name ? ` ${o.name}` : ''}${o.siteName ? ` (${o.siteName})` : ''}`
  if (o.cidr) return `${o.cidr} ${o.name}`
  if (o.email) return o.displayName ? `${o.displayName} (${o.email})` : o.email
  if (o.reason !== undefined) return `${o.reason}${o.expired ? ' (expired)' : ''}`
  if (o.deviceName) return `${o.name} on ${o.deviceName}`
  return o.name
}

/** URL-backed filters: they survive refresh, and changing one resets the page. */
export function useUrlFilters() {
  const [params, setParams] = useSearchParams()
  /** A filter change resets the page; pass resetPage false for view state such as an open record. */
  const set = (key: string, value: string, resetPage = true) => setParams((prev) => {
    const next = new URLSearchParams(prev)
    if (value) next.set(key, value)
    else next.delete(key)
    if (resetPage) next.delete('page')
    return next
  })
  const replaceAll = (values: Record<string, string>, keep: string[] = []) => setParams((prev) => {
    const next = new URLSearchParams()
    for (const k of keep) {
      const v = prev.get(k)
      if (v) next.set(k, v)
    }
    for (const [k, v] of Object.entries(values)) if (v) next.set(k, v)
    return next
  })
  return { params, get: (key: string) => params.get(key) ?? '', set, replaceAll }
}

/** Remembers which optional columns a viewer hides, per table, in this browser only. */
export function useColumns(table: string, all: string[], hiddenByDefault: string[] = []) {
  const key = `dfipam.columns.${table}`
  const [hidden, setHidden] = useState<string[]>(() => {
    try {
      const raw = localStorage.getItem(key)
      if (raw) return (JSON.parse(raw) as string[]).filter((c) => all.includes(c))
    } catch {
      // Storage can be unavailable; fall back to the defaults.
    }
    return hiddenByDefault
  })
  const toggle = (column: string) => setHidden((prev) => {
    const next = prev.includes(column) ? prev.filter((c) => c !== column) : [...prev, column]
    try { localStorage.setItem(key, JSON.stringify(next)) } catch { /* not saved, still applied */ }
    return next
  })
  return { visible: (column: string) => !hidden.includes(column), hidden, toggle }
}
