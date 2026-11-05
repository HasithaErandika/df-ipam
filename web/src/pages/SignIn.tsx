import { zodResolver } from '@hookform/resolvers/zod'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { z } from 'zod'
import { SubnetMap } from '../components/SubnetMap'
import { Button, Field, FormError, Input } from '../components/ui'
import { signIn } from '../lib/auth'

const schema = z.object({
  email: z.email('Enter a valid email address.'),
  password: z.string().min(1, 'Enter your password.'),
})

export function SignIn() {
  const [error, setError] = useState<unknown>(null)
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
          <p className="font-mono text-[13px] text-map-accent">10.0.0.0/8 · 2001:db8::/32</p>
          <h2 className="mt-3 max-w-md text-3xl leading-tight font-semibold tracking-tight text-map-ink">
            Every network, address and owner in one governed record.
          </h2>
        </div>
      </div>
      <main className="flex w-full items-center justify-center bg-surface px-6 lg:w-[460px] lg:shrink-0">
        <div className="w-full max-w-sm">
          <h1 className="text-[22px] font-semibold tracking-tight text-ink">Sign in to DF-IPAM</h1>
          <p className="mt-1 text-ink-2">Dialog Finance IP Address Management</p>
          <form onSubmit={submit} noValidate className="mt-7 space-y-4">
            <Field label="Work email" htmlFor="email" error={errors.email?.message}>
              <Input id="email" type="email" autoComplete="username" autoFocus {...register('email')} />
            </Field>
            <Field label="Password" htmlFor="password" error={errors.password?.message}>
              <Input id="password" type="password" autoComplete="current-password" {...register('password')} />
            </Field>
            <FormError error={error} />
            <Button type="submit" variant="primary" busy={isSubmitting} className="w-full">Sign in</Button>
          </form>
          <p className="mt-6 text-[13px] text-ink-3">Access is by invitation. Contact your IPAM administrator if you need an account.</p>
        </div>
      </main>
    </div>
  )
}
