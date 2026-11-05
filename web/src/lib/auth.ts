// The only file that knows about Supabase. It is used for sign-in only: data
// always goes through the DF-IPAM API. Moving to company auth replaces this file.
import { createClient } from '@supabase/supabase-js'

const client = createClient(import.meta.env.VITE_SUPABASE_URL, import.meta.env.VITE_SUPABASE_ANON_KEY, {
  auth: { flowType: 'pkce', persistSession: true, autoRefreshToken: true },
})

export async function signIn(email: string, password: string): Promise<void> {
  const { error } = await client.auth.signInWithPassword({ email, password })
  if (error) throw new Error(error.status === 400 ? 'Incorrect email or password.' : error.message)
}

export async function signOut(): Promise<void> {
  await client.auth.signOut()
}

export async function getAccessToken(): Promise<string | null> {
  const { data } = await client.auth.getSession()
  return data.session?.access_token ?? null
}

export function onAuthChange(callback: (signedIn: boolean) => void): () => void {
  const { data } = client.auth.onAuthStateChange((_event, session) => callback(session !== null))
  return () => data.subscription.unsubscribe()
}
