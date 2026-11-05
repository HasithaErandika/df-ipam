import { MutationCache, QueryCache, QueryClient } from '@tanstack/react-query'
import { toast } from '../components/ui'
import { ApiError, errorMessage } from '../lib/api'
import { signOut } from '../lib/auth'

declare module '@tanstack/react-query' {
  interface Register {
    mutationMeta: {
      /** Said when it works. A function can use what was sent and what came back. */
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      success?: string | ((variables: any, data: any) => string | undefined)
      /** The screen shows this action's error itself, so no toast is needed. */
      inlineError?: boolean
    }
  }
}

/** A session that has ended sends the person back to sign in. */
function endSession(error: unknown): boolean {
  if (!(error instanceof ApiError) || error.status !== 401) return false
  toast.error(errorMessage(error))
  void signOut().then(() => queryClient.clear())
  return true
}

/** Asking again can't fix a request the server refused. */
function shouldRetry(failures: number, error: unknown): boolean {
  const refused = error instanceof ApiError && error.status >= 400 && error.status < 500
  return failures < 1 && !refused
}

export const queryClient: QueryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, retry: shouldRetry, refetchOnWindowFocus: false } },
  queryCache: new QueryCache({
    onError: (error, query) => {
      // A first load shows its error in place. A failed refresh would otherwise go unnoticed.
      if (!endSession(error) && query.state.data !== undefined) toast.error(errorMessage(error))
    },
  }),
  mutationCache: new MutationCache({
    onSuccess: (data, variables, _context, mutation) => {
      const success = mutation.meta?.success
      const message = typeof success === 'function' ? success(variables, data) : success
      if (message) toast.success(message)
    },
    onError: (error, _variables, _context, mutation) => {
      if (!endSession(error) && !mutation.meta?.inlineError) toast.error(errorMessage(error))
    },
  }),
})
