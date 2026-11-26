import { useQuery } from '@tanstack/react-query'
import { useLocation, useNavigate, useSearchParams } from 'react-router'
import { list } from './api'

/**
 * An open dialog kept in the query string (?device=<id>), so it can be linked
 * to and the back button closes it. Opening adds a history entry; closing one
 * opened here goes back to it, so history doesn't fill with copies of the list.
 */
export function useDialogParam(name: string): [string | null, (id: string | null) => void] {
  const [params, setParams] = useSearchParams()
  const location = useLocation()
  const navigate = useNavigate()
  const openedHere = (location.state as { dialog?: string } | null)?.dialog === name
  const set = (id: string | null) => {
    if (!id && openedHere) {
      void navigate(-1)
      return
    }
    setParams((prev) => {
      const next = new URLSearchParams(prev)
      if (id) next.set(name, id)
      else next.delete(name)
      return next
    }, { replace: !id, state: id ? { dialog: name } : null })
  }
  return [params.get(name), set]
}

/**
 * The record a details dialog shows: the row on this page when it is there,
 * otherwise fetched by id, so a shared link opens whatever page or filter is
 * showing. The key starts with the list path, so the list's refreshes reach it.
 */
export function useOpenRecord<T extends { id: string }>(path: string, openId: string | null, rows: T[]): T | null {
  const row = openId ? rows.find((r) => r.id === openId) : undefined
  const one = useQuery({
    queryKey: [path, 'one', openId],
    queryFn: () => list<T>(`${path}?id=${encodeURIComponent(openId!)}`),
    enabled: Boolean(openId) && !row,
  })
  return row ?? (openId ? one.data?.data[0] ?? null : null)
}
