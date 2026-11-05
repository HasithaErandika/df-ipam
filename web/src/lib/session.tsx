import { useQuery } from '@tanstack/react-query'
import { createContext, use, useEffect, useState, type ReactNode } from 'react'
import { get } from './api'
import { getAccessToken, onAuthChange } from './auth'
import type { Me } from './types'

export function useSignedIn(): boolean | null {
  const [signedIn, setSignedIn] = useState<boolean | null>(null)
  useEffect(() => {
    getAccessToken().then((token) => setSignedIn(token !== null))
    return onAuthChange(setSignedIn)
  }, [])
  return signedIn
}

export const useMeQuery = () => useQuery({ queryKey: ['me'], queryFn: () => get<Me>('/auth/me'), retry: false })

const MeContext = createContext<Me | null>(null)

export function MeProvider({ me, children }: { me: Me; children: ReactNode }) {
  return <MeContext value={me}>{children}</MeContext>
}

export function useMe(): Me {
  const me = use(MeContext)
  if (!me) throw new Error('useMe needs MeProvider')
  return me
}

export function useCan(permission: string): boolean {
  return useMe().permissions.includes(permission)
}
