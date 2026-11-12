import { useSearchParams } from 'react-router'

/**
 * An open dialog kept in the query string (?device=<id>), so it can be linked
 * to and the back button closes it.
 */
export function useDialogParam(name: string): [string | null, (id: string | null) => void] {
  const [params, setParams] = useSearchParams()
  const set = (id: string | null) => setParams((prev) => {
    const next = new URLSearchParams(prev)
    if (id) next.set(name, id)
    else next.delete(name)
    return next
  }, { replace: !id })
  return [params.get(name), set]
}
