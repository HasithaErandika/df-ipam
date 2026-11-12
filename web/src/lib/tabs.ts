import { useParams } from 'react-router'

/**
 * A page's tabs are paths, such as /reconciliation/changes, so each can be linked to
 * and the back button moves between them. Filters and paging stay in the query string.
 * A tab named after its page lives at the page's own path, as in /devices.
 */
export function tabPath(base: string, key: string): string {
  return key === base.slice(1) ? base : `${base}/${key}`
}

/** The open tab. An address naming a tab this person can't see opens the first one. */
export function useRouteTab(keys: string[]): string {
  const { tab } = useParams()
  return tab && keys.includes(tab) ? tab : keys[0]
}
