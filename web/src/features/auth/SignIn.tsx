import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { SubnetMap } from '../../components/SubnetMap'
import { Button, Field, FormError, Input, Notice } from '../../components/ui'
import { authStatus, signIn, signInWithMicrosoft, type AuthStatus } from '../../lib/auth'
import { takeSignOutReason } from '../../lib/idle'

const schema = z.object({
  email: z.email('Enter a valid email address.'),
  password: z.string().min(1, 'Enter your password.'),
})

/** Why Microsoft sign-in sent the person back, from /?signin_error=. */
const refused: Record<string, string> = {
  domain: 'Use your @dialogfinance.lk Microsoft account. Other accounts, including guests, can\'t sign in.',
  tenant: 'That Microsoft account belongs to another organisation. Use your Dialog Finance account.',
  disabled: 'Your DF-IPAM access has been switched off. Ask your IPAM administrator.',
  expired: 'The sign-in took too long or was opened in another tab. Try again.',
  cancelled: 'Sign-in was cancelled.',
  off: 'Microsoft sign-in isn\'t set up on this server.',
  failed: 'Microsoft sign-in didn\'t complete. Try again, and tell your IPAM administrator if it keeps happening.',
}

function takeRefusal(): string | null {
  const params = new URLSearchParams(window.location.search)
  const code = params.get('signin_error')
  if (!code) return null
  params.delete('signin_error')
  window.history.replaceState(null, '', window.location.pathname + (params.size ? `?${params}` : ''))
  return refused[code] ?? refused.failed
}

function MicrosoftMark() {
  return (
    <svg width="16" height="16" viewBox="0 0 21 21" aria-hidden>
      <rect x="1" y="1" width="9" height="9" fill="#f25022" /><rect x="11" y="1" width="9" height="9" fill="#7fba00" />
      <rect x="1" y="11" width="9" height="9" fill="#00a4ef" /><rect x="11" y="11" width="9" height="9" fill="#ffb900" />
    </svg>
  )
}

export function SignIn() {
  const [refusal] = useState(takeRefusal)
  const [status, setStatus] = useState<AuthStatus | null>(null)
  const [leaving, setLeaving] = useState(false)
  useEffect(() => { void authStatus().then(setStatus) }, [])
  const [error, setError] = useState<unknown>(null)
  const [reason] = useState(takeSignOutReason)
  const { register, handleSubmit, formState: { errors, isSubmitting } } = useForm({ resolver: zodResolver(schema) })

  const submit = handleSubmit(async ({ email, password }) => {
    setError(null)
    try {
      await signIn(email, password)
    } catch (e) {
      setError(e)
    }
  })

  return (
    <div className="flex h-full">
      <div className="relative hidden flex-1 overflow-hidden bg-map lg:block">
        <SubnetMap />
        <div className="absolute inset-0 bg-gradient-to-t from-map via-map/70 to-map/10" />
        <div className="absolute inset-x-0 bottom-0 p-12">
          <p className="font-mono text-[13px] text-map-accent">192.168.0.0/16 · 172.16.0.0/12</p>
          <h2 className="mt-3 max-w-md text-3xl leading-tight font-semibold tracking-tight text-map-ink">
            Every floor, server and device on the network, in one record.
          </h2>
        </div>
      </div>
      <main className="flex w-full items-center justify-center bg-surface px-6 lg:w-[460px] lg:shrink-0">
        <div className="w-full max-w-sm">
          <h1 className="text-[22px] font-semibold tracking-tight text-ink">Sign in to DF-IPAM</h1>
          <p className="mt-1 text-ink-2">Dialog Finance IP Address Management</p>
          {reason && <div className="mt-5"><Notice tone="warn">{reason}</Notice></div>}
          {refusal && <div className="mt-5"><Notice tone="danger">{refusal}</Notice></div>}
          {status?.microsoft && (
            <>
              <Button variant="primary" className="mt-7 h-10 w-full gap-2.5" busy={leaving} onClick={() => { setLeaving(true); signInWithMicrosoft() }}>
                {!leaving && <MicrosoftMark />}Sign in with Microsoft
              </Button>
              <p className="mt-2 text-center text-[12.5px] text-ink-3">Use your @dialogfinance.lk account.</p>
              <div className="my-6 flex items-center gap-3 text-[12px] text-ink-3"><span className="h-px flex-1 bg-line" />or with email and password<span className="h-px flex-1 bg-line" /></div>
            </>
          )}
          <form onSubmit={submit} noValidate className={status?.microsoft ? 'space-y-4' : 'mt-7 space-y-4'}>
            <Field label="Work email" htmlFor="email" error={errors.email?.message}>
              <Input id="email" type="email" autoComplete="username" autoFocus {...register('email')} />
            </Field>
            <Field label="Password" htmlFor="password" error={errors.password?.message}>
              <Input id="password" type="password" autoComplete="current-password" {...register('password')} />
            </Field>
            <FormError error={error} />
            <Button type="submit" variant={status?.microsoft ? 'secondary' : 'primary'} busy={isSubmitting} className="w-full">Sign in</Button>
          </form>
          <p className="mt-6 text-[13px] text-ink-3">Access is by invitation. Contact your IPAM administrator if you need an account.</p>
        </div>
      </main>
    </div>
  )
}
