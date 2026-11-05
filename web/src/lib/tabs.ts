import { useParams } from 'react-router'

/**
 * A page's tabs are paths, such as /reconciliation/changes, so each can be linked to
 * and the back button moves between them. Filters and paging stay in the query string.
 * A tab named after its page lives at the page's own path, as in /devices.
 */
export function tabPath(base: string, key: string): string {
  return key === base.slice(1) ? base : `${base}/${key}`
}

/** The address an old ?tab= link now lives at, or null. Stored notifications still hold these. */
export function legacyTabUrl(pathname: string, search: string): string | null {
  const params = new URLSearchParams(search)
  const tab = params.get('tab')
  if (!tab) return null
  params.delete('tab')
  const rest = params.toString()
  return `${tabPath(pathname, tab)}${rest ? `?${rest}` : ''}`
}

/** The open tab. An address naming a tab this person can't see opens the first one. */
export function useRouteTab(keys: string[]): string {
  const { tab } = useParams()
  return tab && keys.includes(tab) ? tab : keys[0]
}
