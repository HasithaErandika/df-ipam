import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router'
import { list, queryString } from './api'
import { useClampPage, usePaging } from './paging'

/**
 * Filters live in the query string, so a filtered view survives refresh, the
 * back button and being shared as a link. Changing a filter goes back to page 1.
 */
export function useFilters<K extends string>(keys: readonly K[]) {
  const [params, setParams] = useSearchParams()
  const values = Object.fromEntries(keys.map((k) => [k, params.get(k) ?? ''])) as Record<K, string>
  const set = (key: K, value: string) => setParams((prev) => {
    const next = new URLSearchParams(prev)
    if (value) next.set(key, value)
    else next.delete(key)
    next.delete('page')
    return next
  }, { replace: true })
  const clear = () => setParams((prev) => {
    const next = new URLSearchParams(prev)
    for (const k of [...keys, 'page']) next.delete(k)
    return next
  }, { replace: true })
  const active = keys.filter((k) => values[k] !== '').length
  return { values, set, clear, active }
}

/** A value that follows another after it stops changing, so typing doesn't send a request per key. */
export function useDebounced<T>(value: T, ms = 300): T {
  const [out, setOut] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setOut(value), ms)
    return () => clearTimeout(t)
  }, [value, ms])
  return out
}

/**
 * One page of a list endpoint. The previous page stays on screen while the next
 * loads, so the table doesn't flash empty between pages.
 */
export function usePagedList<T>(path: string, filters: Record<string, string | undefined>, options: { enabled?: boolean; refetchInterval?: number; prefix?: string } = {}) {
  const paging = usePaging(options.prefix ?? '', 25)
  const qs = queryString({ ...filters, page: paging.page, pageSize: paging.pageSize })
  const query = useQuery({
    queryKey: [path, qs],
    queryFn: () => list<T>(`${path}${qs}`),
    placeholderData: keepPreviousData,
    enabled: options.enabled,
    refetchInterval: options.refetchInterval,
  })
  useClampPage(paging, query.data?.meta)
  return { query, paging, rows: query.data?.data ?? [], meta: query.data?.meta }
}
