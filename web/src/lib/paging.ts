import { useEffect } from 'react'
import { useSearchParams } from 'react-router'
import type { PageMeta } from './api'

export interface Paging {
  page: number
  pageSize: number
  setPage: (page: number) => void
  setPageSize: (size: number) => void
}

const positive = (value: string | null, fallback: number) => {
  const n = Number(value)
  return Number.isInteger(n) && n > 0 ? n : fallback
}

/**
 * Keeps page and page size in the URL, so they survive refresh and the back button.
 * A prefix lets two tables share one page.
 */
export function usePaging(prefix = '', defaultSize = 20): Paging {
  const [params, setParams] = useSearchParams()
  const pageKey = prefix ? `${prefix}Page` : 'page'
  const sizeKey = prefix ? `${prefix}PageSize` : 'pageSize'
  const change = (update: (next: URLSearchParams) => void, replace = false) =>
    setParams((prev) => {
      const next = new URLSearchParams(prev)
      update(next)
      return next
    }, { replace })
  return {
    page: positive(params.get(pageKey), 1),
    pageSize: positive(params.get(sizeKey), defaultSize),
    setPage: (page) => change((next) => {
      if (page > 1) next.set(pageKey, String(page))
      else next.delete(pageKey)
    }),
    setPageSize: (size) => change((next) => {
      next.set(sizeKey, String(size))
      next.delete(pageKey) // the old page number means nothing at a new size
    }),
  }
}

/** Moves back to the last real page when rows were removed or a filter narrowed the list. */
export function useClampPage(paging: Paging, meta: PageMeta | undefined) {
  const { page, setPage } = paging
  const last = meta?.totalPages
  useEffect(() => {
    if (last !== undefined && page > 1 && page > Math.max(last, 1)) setPage(Math.max(last, 1))
    // setPage changes every render; only the numbers should trigger this.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, last])
}
