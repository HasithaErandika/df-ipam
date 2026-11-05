import { useQueryClient } from '@tanstack/react-query'

/** Returns a function that marks every query under these keys as out of date. */
export function useInvalidate(...keys: string[]) {
  const queryClient = useQueryClient()
  return () => { for (const key of keys) queryClient.invalidateQueries({ queryKey: [key] }) }
}
