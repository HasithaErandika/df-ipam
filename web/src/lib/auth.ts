// How people sign in, and the only file that knows. Two ways while both run:
// Microsoft Entra ID, where the API holds the session in a cookie scripts can't
// read, and Supabase email and password, where supabase-js holds a token. Data
// always goes through the DF-IPAM API. Removing Supabase removes its half here.
import { createClient } from '@supabase/supabase-js'

const client = createClient(import.meta.env.VITE_SUPABASE_URL, import.meta.env.VITE_SUPABASE_ANON_KEY, {
  auth: { flowType: 'pkce', persistSession: true, autoRefreshToken: true },
})

const api = `${import.meta.env.VITE_API_URL ?? ''}/api/v1`

/** Which ways in are switched on, and whether this browser holds a Microsoft session. */
export interface AuthStatus { microsoft: boolean; password: boolean; session: boolean }

export async function authStatus(): Promise<AuthStatus> {
  try {
    const res = await fetch(`${api}/auth/status`, { credentials: 'include' })
    if (res.ok) return await res.json()
  } catch {
    // The API is down: the sign-in page still offers email and password.
  }
  return { microsoft: false, password: true, session: false }
}

/** Leaves for Microsoft's sign-in page; the API brings the person back to returnTo. */
export function signInWithMicrosoft(returnTo = window.location.pathname + window.location.search): void {
  const back = returnTo.startsWith('/') && !returnTo.startsWith('//') ? returnTo : '/'
  window.location.assign(`${api}/auth/microsoft/login?returnTo=${encodeURIComponent(back)}`)
}

export async function signIn(email: string, password: string): Promise<void> {
  const { error } = await client.auth.signInWithPassword({ email, password })
  if (error) throw new Error(error.status === 400 ? 'Incorrect email or password.' : error.message)
}

let microsoftSession = false
const listeners = new Set<(signedIn: boolean) => void>()

/** Ends whichever session this browser holds. */
export async function signOut(): Promise<void> {
  if (microsoftSession) {
    try {
      await fetch(`${api}/auth/logout`, { method: 'POST', credentials: 'include', headers: { 'X-Requested-With': 'DF-IPAM' } })
    } catch {
      // The cookie expires on its own; the screen signs out regardless.
    }
    microsoftSession = false
    listeners.forEach((l) => l(false))
  }
  await client.auth.signOut()
}

/** A Supabase token for the Authorization header, or null with a Microsoft session (the cookie goes by itself). */
export async function getAccessToken(): Promise<string | null> {
  const { data } = await client.auth.getSession()
  return data.session?.access_token ?? null
}

/** Whether anyone is signed in, either way. */
export async function isSignedIn(): Promise<boolean> {
  if (await getAccessToken()) return true
  microsoftSession = (await authStatus()).session
  return microsoftSession
}

export function onAuthChange(callback: (signedIn: boolean) => void): () => void {
  listeners.add(callback)
  const { data } = client.auth.onAuthStateChange((_event, session) => { if (!microsoftSession) callback(session !== null) })
  return () => { listeners.delete(callback); data.subscription.unsubscribe() }
}
