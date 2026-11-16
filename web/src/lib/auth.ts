// How people sign in, and the only file that knows. The API holds the session
// in a cookie scripts can't read; the browser never sees a token. Two ways in:
// Microsoft Entra ID, and the development sign-in while Entra ID isn't set up.

const api = `${import.meta.env.VITE_API_URL ?? ''}/api/v1`
const csrf = { 'X-Requested-With': 'DF-IPAM' }

/** Which ways in are switched on, and whether this browser holds a session. */
export interface AuthStatus { microsoft: boolean; bypass: boolean; session: boolean }

const unknownStatus: AuthStatus = { microsoft: false, bypass: false, session: false }

export async function authStatus(): Promise<AuthStatus> {
  try {
    const res = await fetch(`${api}/auth/status`, { credentials: 'include' })
    if (res.ok) return { ...unknownStatus, ...(await res.json()) }
  } catch {
    // The API is down: the sign-in page says so.
  }
  return unknownStatus
}

/** Leaves for Microsoft's sign-in page; the API brings the person back to returnTo. */
export function signInWithMicrosoft(returnTo = window.location.pathname + window.location.search): void {
  const back = returnTo.startsWith('/') && !returnTo.startsWith('//') ? returnTo : '/'
  window.location.assign(`${api}/auth/microsoft/login?returnTo=${encodeURIComponent(back)}`)
}

const listeners = new Set<(signedIn: boolean) => void>()

/** The development sign-in: a session as the person set in api/.env, without a password. */
export async function signInWithBypass(): Promise<void> {
  let res: Response
  try {
    res = await fetch(`${api}/auth/bypass`, { method: 'POST', credentials: 'include', headers: csrf })
  } catch {
    throw new Error("Can't reach the server. Check your connection and try again.")
  }
  if (!res.ok) {
    const body = await res.json().catch(() => null)
    throw new Error(body?.error?.message ?? 'The development sign-in did not work.')
  }
  listeners.forEach((l) => l(true))
}

/** Ends this browser's session. */
export async function signOut(): Promise<void> {
  try {
    await fetch(`${api}/auth/logout`, { method: 'POST', credentials: 'include', headers: csrf })
  } catch {
    // The cookie expires on its own; the screen signs out regardless.
  }
  listeners.forEach((l) => l(false))
}

/** Whether this browser holds a live session. */
export async function isSignedIn(): Promise<boolean> {
  return (await authStatus()).session
}

export function onAuthChange(callback: (signedIn: boolean) => void): () => void {
  listeners.add(callback)
  return () => { listeners.delete(callback) }
}
